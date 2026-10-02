#!/usr/bin/env python3
"""joomscan — неинвазивный аудит Joomla-сайта (обёртка над CLI joomscan).

Вход (stdin JSON): url (единственная цель), components (опц., bool), timeout
(опц., 60), wall_timeout (опц., общий лимит, 300).

Вызов: <JOOMSCAN_BIN|joomscan> -u <url> [--enumerate-components] --timeout N
       (cwd = временная папка).

Точка входа. JoomScan (OWASP/joomscan) — ПЕРЛ-скрипт, не пакет PyPI: `pip install
joomscan` не существует (404 на PyPI), `python -m joomscan` тоже нет. Реальная
точка входа — сам скрипт: в Kali/Debian это бинарь `joomscan` (apt install
joomscan), из репозитория — joomscan.pl на Perl. Поэтому resolve_bin ищет
`joomscan`, затем `joomscan.pl` в PATH и НЕ подставляет `python -m`. Флаги сверены
с core/header.pl (joomscan 0.0.7): -u/--url, --enumerate-components (короткая
-ec), --no-report/-nr, --timeout, -jv/--joomla-version, -r/--random-agent.
Флага --force у joomscan НЕТ, а --enumerate без -components не существует — ничего
не выдумываем. Свою разведку не запускаем: одна цель, заданная оператором.

Отчёт: core/report.pl пишет в CWD reports/<host>/<host>_report_<дата>_at_<время>.txt
(и .html) — берём txt. Формат: dprint печатает "\n[+] <проверка>\n", tprint/fprint
"[++] <детали>"; в txt обе метки одинаковые, поэтому находки отделяем по тексту:
VULN_NEGATIVE вычитается первым, потом ищем VULN_POSITIVE — это подстроки реальных
сообщений модулей joomscan (core/ver.pl, exploit/verexploit.pl, jckeditor.pl,
com_lfd.pl, modules/{pathdisclure,debugmode,dirlisting,missconfig,backupfinder,
robots,reg,cpfinder,waf_detector}.pl). Остальное молча игнорируем — разбор
намеренно консервативный. Версия: блок вида "Joomla3.9.24" (core/ver.pl печатает
$ver через tr, пробелов в нём нет). Компоненты: строки "Name: com_content" +
"Location : <url>" из exploit/components.pl.

Выход (stdout JSON): {url, version, vulns[{check,detail}], components[{name,
location}], count}; count = len(vulns). Ничего не нашлось — ok с пустыми массивами.
Доменные ошибки: empty_url, bad_url, bad_components, bad_timeout,
joomscan_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечисловой wall_timeout, нечитаемый
репорт (bad_report).
"""
import glob
import json
import os
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
except Exception:
    pass

DEFAULT_TIMEOUT = 60
DEFAULT_WALL = 300

VULN_NEGATIVE = ("not vulnerable", "not found", "not detected", "not alive",
                 "cannot ensure", "ver 404")

VULN_POSITIVE = ("vulnerable", "sql injection", "local file disclosure",
                 "full path disclosure", "directory listing",
                 "debug mode enabled", "backup file is found",
                 "interesting file is found", "is found", "admin page :",
                 "registration is enabled", "we found the component")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def as_bool(value, default):
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        low = value.strip().lower()
        if low in ("1", "true", "yes", "on"):
            return True
        if low in ("", "0", "false", "no", "off"):
            return False
        return default
    return default


def number_or_none(value):
    if value is None or value == "" or isinstance(value, bool):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def resolve_bin():
    """Путь к скрипту joomscan или None — модуля у донора нет, искать нечего."""
    bin_env = os.environ.get("JOOMSCAN_BIN", "").strip()
    if bin_env:
        if "/" not in bin_env and "\\" not in bin_env:
            return shutil.which(bin_env) or os.path.abspath(bin_env)
        return os.path.abspath(bin_env)
    for name in ("joomscan", "joomscan.pl"):
        found = shutil.which(name)
        if found:
            return found
    return None


def split_blocks(text):
    """Отчёт joomscan -> список (заголовок проверки, строки блока [++])."""
    blocks = []
    check = ""
    detail = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line:
            continue
        if line.startswith("[+]"):
            if detail:
                blocks.append((check, detail))
                detail = []
            check = line[3:].strip()
            continue
        if line.startswith("[++]"):
            if detail:
                blocks.append((check, detail))
                detail = []
            detail = [line[4:].strip()]
            continue
        if detail:
            detail.append(line)
    if detail:
        blocks.append((check, detail))
    return blocks


def is_vuln(text):
    low = text.lower()
    if any(bad in low for bad in VULN_NEGATIVE):
        return False
    return any(good in low for good in VULN_POSITIVE)


def parse_report(text):
    version = ""
    vulns = []
    components = []
    seen_vulns = set()
    seen_components = set()
    for check, lines in split_blocks(text):
        block = "\n".join(lines).strip()
        head = lines[0] if lines else ""
        low = head.lower()
        if not version and low.startswith("joomla"):
            tail = low[len("joomla"):].strip(" .:")
            if tail and tail[0].isdigit():
                version = tail
        if not version and len(lines) == 1 and low.replace(".", "").isdigit():
            version = head.strip()
        for line in lines:
            if line.lower().startswith("name:") and "com_" in line:
                name = line.split(":", 1)[1].strip().split()[0]
                location = ""
                for other in lines[1:]:
                    if other.lower().startswith("location"):
                        location = other.split(":", 1)[1].strip()
                        break
                key = (name, location)
                if name and key not in seen_components:
                    seen_components.add(key)
                    components.append({"name": name, "location": location})
                break
        if is_vuln(block):
            key = (check, head)
            if key not in seen_vulns:
                seen_vulns.add(key)
                vulns.append({"check": check, "detail": head})
    return version, vulns, components


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")
    if "://" not in url:
        url = "http://" + url
    if not url.startswith(("http://", "https://")):
        return fail("bad_url", f"схема не http(s): {url}")

    if data.get("components") is not None and not isinstance(
            data.get("components"), (bool, str, int, float)):
        return fail("bad_components", "components должен быть boolean")
    enumerate_components = as_bool(data.get("components"), False)

    timeout = number_or_none(data.get("timeout"))
    if timeout is None and data.get("timeout") not in (None, ""):
        return fail("bad_timeout", "timeout должен быть числом")
    if timeout is not None and timeout < 1:
        return fail("bad_timeout", "timeout должен быть >= 1")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    binary = resolve_bin()
    if binary is None:
        return fail("joomscan_not_installed",
                    "joomscan не найден в PATH: apt install joomscan (Kali) "
                    "или git clone https://github.com/OWASP/joomscan "
                    "(либо укажите JOOMSCAN_BIN путём к joomscan/joomscan.pl)")
    cmd = [binary]
    if binary.lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable] + cmd
    cmd += ["-u", url, "--timeout",
            str(int(timeout if timeout is not None else DEFAULT_TIMEOUT))]
    if enumerate_components:
        cmd.append("--enumerate-components")

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("joomscan_not_installed",
                        "joomscan не найден: apt install joomscan "
                        "(или укажите JOOMSCAN_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"joomscan не уложился в {wall:.0f}s: уменьшите "
                        "нагрузку или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        matches = sorted(glob.glob(os.path.join(td, "reports", "*",
                                                "*_report_*.txt")))
        if not matches:
            matches = sorted(glob.glob(os.path.join(td, "reports", "*",
                                                    "*.txt")))
        if not matches:
            combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
            tail = combined.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"joomscan упал (exit {proc.returncode}): {last}")
            if "not alive" in combined.lower():
                return fail("tool_failed", f"joomscan: цель не отвечает: {last}")
            return fail("no_report",
                        f"joomscan не дал текстовый репорт: {last}")
        report_path = max(matches, key=os.path.getmtime)
        try:
            with open(report_path, encoding="utf-8") as f:
                text = f.read()
        except Exception as e:
            return fail("bad_report", f"не прочитан репорт joomscan: {e}",
                        exit_code=2)

    if not text.strip():
        return fail("no_report", "репорт joomscan пуст")

    version, vulns, components = parse_report(text)
    return ok({"url": url, "version": version, "vulns": vulns,
               "components": components, "count": len(vulns)})


if __name__ == "__main__":
    sys.exit(main())