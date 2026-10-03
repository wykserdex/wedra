#!/usr/bin/env python3
"""nosqlmap — проверка веб-приложения на NoSQL-инъекции (CLI nosqlmap).

Аудит ровно той цели, что дал пользователь: только web-app атака (--attack 2) на
одном host/port/uri, без сканера анонимного доступа (--attack 3), без атак на
NoSQL-порт напрямую (--attack 1) и без шеллов/эксFILтрации. Аутентификации и
POST-атак нет: только GET-векторы из query-строки.

Вход (stdin JSON): url (обязателен, с query-параметрами — nosqlmap без них
сразу выходит), parameters (опц., имена query-параметров; пусто — все),
wall_timeout (опц., общий лимит, 300).

Вызов: <NOSQLMAP_BIN|nosqlmap.py> --attack 2 --victim <host> --webPort <port>
       --uri <path?query> --https ON|OFF --httpMethod GET --params <1,2>
       --injectSize 4 --injectFormat 2 --doTimeAttack y --savePath report.txt
       (cwd = временная папка, stdin = /dev/null). Флагов -u/--report/
       --technique у nosqlmap нет; --params ждёт НОМЕРА (с единицы)
       query-параметров, поэтому имена из входа переводятся в номера по разбору
       url. --doTimeAttack передаётся обязательно: nsmweb.getApps() читает
       args.doTimeAttack и сразу зовёт .lower() без проверки на None, поэтому
       без флага донор падает с AttributeError ДО nsmweb.save_to() и файла
       отчёта не бывает. --params тоже обязателен по той же причине (иначе
       None.split() в buildUri()). stdin закрыт потому, что на успешной
       инъекции донор доходит до безусловного raw_input() («MongoDB < 2.4
       detected…») и иначе висит до wall_timeout.

Отчёт — текстовый файл, который пишет nsmweb.save_to():
  Vulnerable URLs:
  Possibly Vulnerable URLs:
  Timing based attacks:
  String Attack-Successful|Unsuccessful
  Integer attack-Successful|Unsuccessful
Имена техник берём из stdout nosqlmap («Test N: <описание>» +
«Successful injection!»/«Possible injection.»), вердикты timing-атак — из
репорта дословно.

Выход (stdout JSON): {url, vulnerable, techniques[], count}. count — сколько
уязвимых URL подтверждено в репорте. Не нашлось — нормальный результат
(vulnerable=false). Доменные ошибки: empty_url, nosqlmap_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый
JSON входа, некорректные поля входа, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import parse_qsl, urlsplit

DEFAULT_WALL = 300
INJECT_SIZE = 4
INJECT_FORMAT = 2
REPORT_NAME = "report.txt"

TEST_RE = re.compile(r"^Test\s+(\d+)\s*:\s*(.+?)\s*$")
SUCCESS_RE = re.compile(r"^\s*String\s+Attack-Successful\s*$", re.MULTILINE)
TIMING_INT_RE = re.compile(r"^\s*Integer\s+attack-Successful\s*$",
                           re.MULTILINE)

try:
    sys.stdin.reconfigure(encoding="utf-8", errors="replace")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


def ok(output):
    print(json.dumps({"status": "ok", "output": output},
                     ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_bin(env_name, default):
    bin_env = os.environ.get(env_name, default).strip() or default
    if "/" in bin_env or "\\" in bin_env:
        return [os.path.abspath(bin_env)]
    return [shutil.which(bin_env) or os.path.abspath(bin_env)]


def report_section(text, header, stop):
    lines = text.splitlines()
    items = []
    inside = False
    for line in lines:
        stripped = line.strip()
        if stripped == header:
            inside = True
            continue
        if inside:
            if stripped in stop:
                break
            if stripped:
                items.append(stripped)
    return items


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
    if not (url.startswith("http://") or url.startswith("https://")):
        return fail("bad_url", "url обязан начинаться с http:// или https://",
                    exit_code=2)

    parts = urlsplit(url)
    if not parts.hostname:
        return fail("bad_url", "в url нет хоста", exit_code=2)
    query_names = [name for name, _ in
                   parse_qsl(parts.query, keep_blank_values=True)]
    if not query_names:
        return fail("bad_url",
                    "nosqlmap проверяет только query-параметры: в url "
                    "не найдено ни одного", exit_code=2)

    raw_params = data.get("parameters")
    if raw_params is not None and not isinstance(raw_params, list):
        return fail("bad_parameters", "parameters обязан быть массивом",
                    exit_code=2)
    wanted = [str(p).strip() for p in (raw_params or []) if str(p).strip()]
    unknown = [p for p in wanted if p not in query_names]
    if unknown:
        return fail("bad_parameters",
                    "параметра нет в query-строке url: " + ", ".join(unknown),
                    exit_code=2)
    if not wanted:
        wanted = list(query_names)
    # nosqlmap ждёт номера параметров с единицы
    indexes = ",".join(str(query_names.index(p) + 1) for p in wanted)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    try:
        port = parts.port or (443 if parts.scheme == "https" else 80)
    except ValueError:
        return fail("bad_url", "порт в url не число", exit_code=2)
    path = parts.path or "/"
    uri = path + ("?" + parts.query if parts.query else "")

    cmd = resolve_bin("NOSQLMAP_BIN", "nosqlmap.py")
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd += ["--attack", "2",
                "--victim", parts.hostname,
                "--webPort", str(port),
                "--uri", uri,
                "--https", "ON" if parts.scheme == "https" else "OFF",
                "--httpMethod", "GET",
                "--params", indexes,
                "--injectSize", str(INJECT_SIZE),
                "--injectFormat", str(INJECT_FORMAT),
                "--doTimeAttack", "y",
                "--savePath", report_path]
        try:
            # stdin в DEVNULL: донор на успешной инъекции зовёт raw_input()
            # без всякой проверки — с унаследованным stdin он ждал бы ввода до
            # wall_timeout вместо того, чтобы отдать отчёт.
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("nosqlmap_not_installed",
                        "nosqlmap не найден: поставьте из исходников "
                        "(git clone https://github.com/codingo/NoSQLMap) и "
                        "укажите путь к nosqlmap.py в NOSQLMAP_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"nosqlmap не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        if not os.path.exists(report_path):
            tail = (stdout or proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"nosqlmap упал с кодом {proc.returncode}: {last}")
            return fail("no_report",
                        f"nosqlmap не дал отчёт ({REPORT_NAME}): {last}")
        if os.path.getsize(report_path) == 0:
            return fail("no_report", f"nosqlmap дал пустой отчёт ({REPORT_NAME})")
        try:
            with open(report_path, encoding="utf-8", errors="replace") as f:
                text = f.read()
        except Exception as e:
            return fail("bad_report", f"не прочитан отчёт nosqlmap: {e}",
                        exit_code=2)

    if "Vulnerable URLs:" not in text:
        return fail("bad_report",
                    "в отчёте nosqlmap нет секции «Vulnerable URLs:»",
                    exit_code=2)

    stop = ("Possibly Vulnerable URLs:", "Timing based attacks:")
    vuln_urls = report_section(text, "Vulnerable URLs:", stop)
    timing = []
    if SUCCESS_RE.search(text):
        timing.append("String Attack-Successful")
    if TIMING_INT_RE.search(text):
        timing.append("Integer attack-Successful")

    techniques = []
    current = ""
    for line in stdout.splitlines():
        match = TEST_RE.match(line.strip())
        if match:
            current = match.group(2).strip()
            continue
        if "Successful injection!" in line or "Possible injection." in line:
            if current and current not in techniques:
                techniques.append(current)
    for verdict in timing:
        if verdict not in techniques:
            techniques.append(verdict)

    return ok({"url": url, "vulnerable": bool(vuln_urls or timing),
               "techniques": techniques, "count": len(vuln_urls)})


if __name__ == "__main__":
    sys.exit(main())