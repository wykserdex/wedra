#!/usr/bin/env python3
"""sqlmap — аудит SQL-инъекций по явно заданной цели (обёртка над CLI sqlmap).

Вход (stdin JSON): url (единственная проверяемая цель), technique (опц., только
B/E/U/T, дефолт "BEU"), wall_timeout (опц., 900).

Вызов: [python3 -m sqlmap | SQLMAP_BIN] -u <url> --batch --flush-session
       --disable-coloring --level=1 --risk=1 --technique=<B/E/U/T>
       --output-dir <временная папка>/output   (cwd = временная папка).

Безопасный аудит: разрушающие техники не включены. Нет --os-shell, --file-write,
--file-read, --sql-shell, --os-pwn, --dump и подобных; stacked queries (S) и
inline (Q) отфильтровываются на входе. --level=1 и --risk=1 — минимальные
нагрузочные настройки, --batch — без вопросов, --flush-session — без опроса
чужих сессий. Сканируется ровно URL из входа: ни --crawl, ни --forms, ни
--scope по домену.

Машинного отчёта у sqlmap нет (признанного формата вывода нет ни в одной
версии), поэтому разбор консервативный, по stdout. Снимаем ANSI-последовательности
и ищем только известные маркеры sqlmap:
  [INFO] testing for SQL injection on <PLACE> parameter '<name>'
  [WARNING] parameter '<name>' does not seem to be injectable
  [WARNING] parameter '<name>' is vulnerable / appears to be SQL injectable
  sqlmap identified ... injection point(s) with payloads:
  Parameter: <name> (<PLACE>) / Payload: <payload> / Type: <technique>
Всё, что не распознано, игнорируется; отсутствие находок — status ok с
vulnerable=false, а не ошибка. Если в stdout/stderr есть символ замены U+FFFD
(вывод не декодируется как UTF-8) — bad_report, exit 2: находки не выдумываются.

Выход (stdout JSON): {url, vulnerable, parameter, injections[{parameter, place,
technique, payload}]}. Доменные ошибки: empty_url, bad_technique,
sqlmap_not_installed, timeout (retryable), no_report (инструмент отработал и не
оставил ни лога, ни строки в своём формате), tool_failed. Платформенные
(exit 2): битый JSON входа, bad_wall_timeout, bad_report.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

LOG_DIR_NAME = "output"
DEFAULT_TECHNIQUE = "BEU"
ALLOWED_TECHNIQUES = set("BEUT")
DEFAULT_WALL = 900

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
LINE_RE = re.compile(r"^\[\d{2}:\d{2}:\d{2}\]\s*\[(\w+)\]\s*(.*)$")
TESTING_RE = re.compile(r"testing (?:for )?SQL injection on (.+?) parameter '([^']+)'")
NOT_INJECTABLE_RE = re.compile(r"parameter '([^']+)' does not seem to be injectable")
VULNERABLE_RE = re.compile(r"parameter '([^']+)' (?:is vulnerable|appears to be SQL injectable)")
POINT_HEADER_RE = re.compile(r"identified .*injection point", re.I)
PARAM_RE = re.compile(r"^Parameter: (.+?)\s*\(([^()]*)\)\s*$")
PAYLOAD_RE = re.compile(r"^Payload: (.+)$")
TYPE_RE = re.compile(r"^Type: (.+)$")

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("SQLMAP_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "sqlmap"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    return cmd


def clean(text):
    return ANSI_RE.sub("", text or "")


def parse_stdout(raw):
    injections = []
    current = None
    vulnerable = False
    saw_sqlmap_line = False

    for line in clean(raw).splitlines():
        line = line.strip()
        if not line:
            continue
        match = LINE_RE.match(line)
        if match:
            saw_sqlmap_line = True
            line = match.group(2).strip()
        if not line:
            continue

        if POINT_HEADER_RE.search(line):
            current = {}
            continue

        match = PARAM_RE.match(line)
        if match and current is not None:
            current["parameter"] = match.group(1).strip()
            current["place"] = match.group(2).strip()
            continue

        match = PAYLOAD_RE.match(line)
        if match and current is not None:
            current["payload"] = match.group(1).strip()
            continue

        match = TYPE_RE.match(line)
        if match and current is not None:
            current["technique"] = match.group(1).strip()
            if current.get("payload") or current.get("technique"):
                injections.append({
                    "parameter": str(current.get("parameter") or ""),
                    "place": str(current.get("place") or ""),
                    "technique": str(current.get("technique") or ""),
                    "payload": str(current.get("payload") or ""),
                })
            current = None
            continue

        if TESTING_RE.search(line) or NOT_INJECTABLE_RE.search(line):
            saw_sqlmap_line = True
        if VULNERABLE_RE.search(line):
            saw_sqlmap_line = True
            vulnerable = True

    if current and (current.get("parameter") or current.get("payload")):
        injections.append({
            "parameter": str(current.get("parameter") or ""),
            "place": str(current.get("place") or ""),
            "technique": str(current.get("technique") or ""),
            "payload": str(current.get("payload") or ""),
        })

    if injections:
        vulnerable = True
    return injections, vulnerable, saw_sqlmap_line


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

    technique = str(data.get("technique") or "").strip().upper() or DEFAULT_TECHNIQUE
    letters = set(technique)
    if not letters <= ALLOWED_TECHNIQUES:
        return fail("bad_technique",
                    f"technique принимает только буквы B, E, U, T "
                    f"(разрушающие S/Q и inline запрещены), получено: {technique}")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    log_file = None
    stdout = ""
    returncode = 0
    stderr = ""
    with tempfile.TemporaryDirectory() as td:
        log_file = os.path.join(td, "sqlmap.log")
        cmd = resolve_bin()
        cmd += ["-u", url, "--batch", "--flush-session", "--disable-coloring",
                "--level=1", "--risk=1", "--technique=" + technique,
                "--output-dir", os.path.join(td, LOG_DIR_NAME)]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  encoding="utf-8", errors="replace",
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("sqlmap_not_installed",
                        "sqlmap не найден: git clone "
                        "https://github.com/sqlmapproject/sqlmap и укажите "
                        "SQLMAP_BIN (или положите клон в PYTHONPATH)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"sqlmap не уложился в {wall:.0f}s: уменьшите набор "
                        "техник или увеличьте wall_timeout", retryable=True)
        stdout = proc.stdout or ""
        stderr = proc.stderr or ""
        if stderr:
            sys.stderr.write(stderr)
        if proc.returncode != 0:
            tail = clean(stdout + stderr).strip().splitlines()
            return fail("tool_failed",
                        f"sqlmap упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")
        if "\ufffd" in stdout or "\ufffd" in stderr:
            return fail("bad_report",
                        "вывод sqlmap не декодируется как UTF-8 — находки "
                        "не разбираются", exit_code=2)
        injections, vulnerable, saw_line = parse_stdout(stdout)
        has_log = os.path.isfile(log_file)
        if not has_log and not saw_line:
            return fail("no_report",
                        "sqlmap не оставил ни лога, ни строк в своём формате")

    return ok({
        "url": url,
        "vulnerable": bool(vulnerable),
        "parameter": injections[0]["parameter"] if injections else "",
        "injections": injections,
    })


if __name__ == "__main__":
    sys.exit(main())
