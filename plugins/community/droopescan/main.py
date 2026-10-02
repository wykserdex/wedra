#!/usr/bin/env python3
"""droopescan — сканер CMS (Drupal и др.), обёртка над CLI droopescan.

Вход (stdin JSON): url, cms (опц.), enumerate (опц., одна буква atpvi),
words (опц., -n), threads (опц., -t), wall_timeout (опц., общий лимит, 600).

Вызов: <DROOPESCAN_BIN|droopescan> scan [cms] -u <url> --output json
       [-e X] [-n N] [-t N]   (cwd = временная папка, stdin = /dev/null).

Флаги сверены с dscan/plugins/internal/scan.py (SamJoan/droopescan 1.33.x,
подкоманда scan, cement-приложение droopescan):
  -u/--url, -U/--url-file, -e/--enumerate (choices: a,t,p,v,i, default a),
  -n/--number (default 1000), -t/--threads (default 4),
  --output/-o — ЭТО ФОРМАТ ВЫВОДА (choices: standard|json), НЕ файл отчёта.
  Флага -f у droopescan нет. Отдельного файла отчёта инструмент не пишет:
  JSON-объект уходит в stdout, и только когда что-то найдено
  (JsonOutput.result печатает лишь при result_anything_found).

Выход (stdout JSON): {url, version, modules[{name,url}], vulns, count}.
version — найденные версии через запятую (дроопскан отдаёт список возможных),
modules — плагины и темы, отсортированы по имени. vulns всегда []: droopescan
не перечисляет уязвимости, сопоставление версии с CVE — за пользователем.
count — всего находок (modules + vulns).
Пустой/текстовый stdout при exit 0 = «ничего не определили» (droopescan так и
сообщает: not identified as a supported CMS) → no_report, а не ok с пустым
массивом: в режиме --output json успешный скан всегда печатает JSON-объект.
Доменные ошибки: empty_url, bad_url, bad_cms, bad_enumerate, bad_words,
bad_threads, droopescan_not_installed, timeout (retryable), tool_failed,
no_report. Платформенные (exit 2): битый JSON входа, битая строка отчёта
(bad_report).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

DEFAULT_WALL = 600
CMS_NAME_CHARS = set("abcdefghijklmnopqrstuvwxyz0123456789_-")
ENUM_CHOICES = ("a", "t", "p", "v", "i")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def int_field(data, name, default, code, minimum=1):
    """(значение, код ошибки): пустое поле → default, не число/мало → код."""
    raw = data.get(name)
    if raw is None or raw == "":
        return default, None
    try:
        value = int(float(raw))
    except (TypeError, ValueError):
        return None, code
    if value < minimum:
        return None, code
    return value, None


def section(report, key):
    """Раздел отчёта droopescan → список находок (пустой список, если нет)."""
    block = report.get(key)
    if not isinstance(block, dict):
        return []
    finds = block.get("finds")
    return finds if isinstance(finds, list) else []


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
        url = "https://" + url
    if not url.startswith(("http://", "https://")):
        return fail("bad_url", f"схема не http(s): {url}")

    cms = str(data.get("cms") or "").strip().lower()
    if cms and (cms[0] == "-" or not set(cms) <= CMS_NAME_CHARS):
        return fail("bad_cms",
                    f"cms должен быть именем CMS буквами/цифрами: {cms}")

    enumerate_mode = str(data.get("enumerate") or "").strip().lower()
    if enumerate_mode and enumerate_mode not in ENUM_CHOICES:
        return fail("bad_enumerate",
                    "enumerate у droopescan — одна буква: a, t, p, v или i "
                    f"(получено {enumerate_mode})")

    words, bad = int_field(data, "words", None, "bad_words")
    if bad:
        return fail(bad, "words должен быть целым >= 1")
    threads, bad = int_field(data, "threads", None, "bad_threads")
    if bad:
        return fail(bad, "threads должен быть целым >= 1")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("DROOPESCAN_BIN", "droopescan").strip()
    # subprocess поедет с cwd во временную папку: имя из PATH ищем which'ем,
    # путь — приводим к абсолютному
    if "/" not in bin_env and "\\" not in bin_env:
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)
    if bin_env.lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable, bin_env]
    else:
        cmd = [bin_env]

    cmd.append("scan")
    if cms:
        cmd.append(cms)
    cmd += ["-u", url, "--output", "json"]
    if enumerate_mode:
        cmd += ["-e", enumerate_mode]
    if words is not None:
        cmd += ["-n", str(words)]
    if threads is not None:
        cmd += ["-t", str(threads)]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("droopescan_not_installed",
                        "droopescan не найден: pip install droopescan "
                        "(или укажите DROOPESCAN_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"droopescan не уложился в {wall:.0f}s: уменьшите "
                        "words или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        raw_out = proc.stdout

    if proc.returncode != 0:
        tail = (proc.stderr or raw_out or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"droopescan упал (exit {proc.returncode}): {last}")

    report = None
    for line in raw_out.splitlines():
        line = line.strip()
        if not line:
            continue
        if not line.startswith("{"):
            continue
        try:
            parsed = json.loads(line)
        except ValueError as e:
            return fail("bad_report",
                        f"строка отчёта droopescan не JSON ({e}): {line[:120]}",
                        exit_code=2)
        if not isinstance(parsed, dict):
            return fail("bad_report",
                        "строка отчёта droopescan не объект: " + line[:120],
                        exit_code=2)
        report = parsed
        break

    if report is None:
        tail = (raw_out or proc.stderr or "").strip().splitlines()
        last = tail[-1] if tail else "пустой вывод"
        return fail("no_report",
                    f"droopescan не выдал JSON-отчёт: {last}")

    version = ",".join(str(v).strip() for v in section(report, "version")
                       if str(v).strip())

    modules = []
    seen = set()
    for key in ("plugins", "themes"):
        for item in section(report, key):
            if not isinstance(item, dict):
                continue
            name = str(item.get("name") or "").strip()
            item_url = str(item.get("url") or "").strip()
            if not name and not item_url:
                continue
            if (name, item_url) in seen:
                continue
            seen.add((name, item_url))
            modules.append({"name": name, "url": item_url})
    modules.sort(key=lambda item: (item["name"], item["url"]))

    vulns = []
    count = len(modules) + len(vulns)
    return ok({"url": url, "version": version, "modules": modules,
               "vulns": vulns, "count": count})


if __name__ == "__main__":
    sys.exit(main())