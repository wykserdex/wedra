#!/usr/bin/env python3
"""Mock CLI sqlmap для контракт-тестов (без сети и без клона sqlmap).

Имитирует `python -m sqlmap -u <url> ... --output-dir <dir>`: печатает в stdout
строки в формате sqlmap "[HH:MM:SS] [LEVEL] сообщение", блок находки — как
настоящий sqlmap (заголовок без префикса, `---`, `Parameter: <name> (<place>)`,
затем на каждую технику `Type:`/`Title:`/`Payload:`), а лог кладёт в
<--output-dir>/<host>/log — туда же, куда его пишет настоящий sqlmap
(lib/core/dump.py, setOutputFile). Режимы env: MOCK_SLEEP=N — спать N секунд
(тест wall_timeout); MOCK_NO_REPORT=1 — не печатать ничего и не писать лог
(тест no_report); MOCK_BAD_REPORT=1 — выдать байты, не декодируемые как UTF-8
(тест bad_report, exit 2); MOCK_CLEAN=1 — ни одной уязвимой точки (ok, count 0);
MOCK_RESUMED=1 — блок, восстановленный из сессии ("resumed ... from stored
session"); MOCK_FAIL=1 — ненулевой код выхода (тест tool_failed).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))


def option_value(argv, names, default=None):
    for i, arg in enumerate(argv):
        if arg in names and i + 1 < len(argv):
            return argv[i + 1]
        for name in names:
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return default


args = sys.argv[1:]
url = option_value(args, ["-u", "--url"])
technique = option_value(args, ["--technique"], "BEUSTQ")
output_dir = option_value(args, ["--output-dir"])

if not url:
    sys.stderr.write("[-] usage: sqlmap.py [options]\n")
    sys.exit(1)

if os.environ.get("MOCK_FAIL") == "1":
    print("[10:00:00] [CRITICAL] Invalid option for --technique", flush=True)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.buffer.write(b"[10:00:00] [INFO] \xff\xfe broken\n")
    sys.stdout.buffer.flush()
    sys.exit(0)

if output_dir:
    os.makedirs(output_dir, exist_ok=True)

log_lines = []


def emit(level, message):
    line = "[10:00:00] [%s] %s" % (level, message)
    print(line, flush=True)
    log_lines.append(line)


def emit_raw(message):
    """Строка без префикса [HH:MM:SS] [LEVEL] — так печатает dumper.string()."""
    print(message, flush=True)
    log_lines.append(message)


def block(parameter, place, entries, resumed=False):
    """Блок _formatInjection: Parameter, затем на каждую технику Type/Title/Payload."""
    if resumed:
        emit_raw("sqlmap resumed the following injection point(s) from stored session")
    else:
        emit_raw("sqlmap identified the following injection point(s) with a "
                 "total of %d HTTP(s) requests:" % (25 * len(entries)))
    emit_raw("---")
    emit_raw("Parameter: %s (%s)" % (parameter, place))
    for technique, title, payload in entries:
        emit_raw("    Type: %s" % technique)
        emit_raw("    Title: %s" % title)
        emit_raw("    Payload: %s" % payload)
        emit_raw("")
    emit_raw("---")


emit("INFO", "testing for SQL injection on GET parameter 'id'")
if os.environ.get("MOCK_RESUMED") == "1":
    block("id", "GET", [("boolean-based blind", "AND 1=1", "id=1 AND 1=1")],
          resumed=True)
elif os.environ.get("MOCK_CLEAN") == "1":
    emit("WARNING", "GET parameter 'id' does not seem to be injectable")
    emit("CRITICAL", "all tested parameters do not appear to be injectable.")
else:
    emit("INFO", "the back-end DBMS is SQLite")
    emit_raw("GET parameter 'id' is vulnerable. Do you want to keep testing "
             "the others (if any)? [y/N] N")
    block("id", "GET", [
        ("boolean-based blind", "AND boolean-based blind - WHERE or HAVING clause",
         "id=1) AND 2184=2184 AND (3334=3334"),
        ("time-based blind", "AND boolean-based blind - WHERE or HAVING clause",
         "id=1) AND (SELECT 2184 FROM (SELECT(SLEEP(5)))a)-- -"),
    ])
    block("name", "POST", [
        ("UNION query", "UNION query - 2 columns", "name=x' UNION SELECT 1,2--"),
    ])
    emit("INFO", "back-end DBMS: MySQL >= 5.1")
    emit("INFO", "SQL injection detected on parameter(s): id, name")

log_dir = output_dir or "."
os.makedirs(os.path.join(log_dir, "demo.invalid"), exist_ok=True)
with open(os.path.join(log_dir, "demo.invalid", "log"), "w", encoding="utf-8") as f:
    f.write("\n".join(log_lines) + "\n")
print(f"[*] finished at 10:00:01, output dir: {output_dir or 'default'}",
      file=sys.stderr)
