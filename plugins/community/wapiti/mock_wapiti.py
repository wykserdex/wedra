#!/usr/bin/env python3
"""Mock CLI wapiti для контракт-тестов (без сети и без пакета wapiti3).

Имитирует wapiti 3.x: читает -u <url>, -m <modules>, --scope <scope>, -f json,
-o <report>, --store-session <dir> и пишет в <report> JSON-отчёт нужной формы
(ровно те пять ключей, что даёт JSONReportGenerator: classifications/
vulnerabilities/anomalies/additionals/infos — никакого suppressed_findings).
Режимы env: MOCK_SLEEP=N — спать N секунд
(тест wall_timeout); MOCK_NO_REPORT=1 — ничего не писать (тест no_report);
MOCK_BAD_REPORT=1 — битый JSON (тест bad_report, exit 2);
MOCK_BAD_SHAPE=1 — JSON-массив вместо объекта (тест bad_report, exit 2);
MOCK_EMPTY=1 — пустой раздел vulnerabilities (нули находок — не ошибка);
MOCK_FAIL=1 — ненулевой код выхода (тест tool_failed).
"""
import json
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
modules = option_value(args, ["-m", "--module"])
scope = option_value(args, ["--scope"], "folder")
report_path = option_value(args, ["-o", "--output"])
fmt = option_value(args, ["-f", "--format"], "html")

if not url:
    sys.stderr.write("[*] Error: --url is required\n")
    sys.exit(2)

print("[*] Wapiti 3.x - a web application vulnerability scanner", file=sys.stderr)
print(f"[*] Target: {url}", file=sys.stderr)
print(f"[*] Scope: {scope}", file=sys.stderr)
print(f"[*] Attack module(s) {modules or 'common'} loaded", file=sys.stderr)

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] Error: module not found", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] Scan finished, no report generated", file=sys.stderr)
    sys.exit(0)

if fmt != "json":
    print("[!] Error: Invalid format", file=sys.stderr)
    sys.exit(2)

if not report_path:
    print("[!] Error: no output path", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(report_path, "w", encoding="utf-8") as f:
        f.write('{"vulnerabilities": {"XSS": [')
    sys.exit(0)

if os.environ.get("MOCK_BAD_SHAPE") == "1":
    with open(report_path, "w", encoding="utf-8") as f:
        json.dump([{"category": "XSS"}], f)
    sys.exit(0)

report = {
    "classifications": {
        "Cross Site Scripting": {"desc": "XSS", "sol": "escape", "ref": [], "wstg": "x"},
    },
    "vulnerabilities": {},
    "anomalies": {},
    "additionals": {},
    "infos": {
        "target": url,
        "scope": scope,
        "version": "Wapiti 3.2.4",
        "detailed_report_level": 0,
        "crawled_pages_nbr": 1,
    },
}

if os.environ.get("MOCK_EMPTY") != "1":
    report["vulnerabilities"] = {
        "Cross Site Scripting": [
            {
                "method": "GET",
                "path": "/search",
                "info": "Reflected cross-site scripting",
                "level": 0,
                "parameter": "q",
                "referer": "",
                "module": "xss",
                "http_request": "GET /search?q=x HTTP/1.1",
                "curl_command": "curl -X 'GET' 'http://target/search?q=x'",
                "wstg": "x",
            }
        ],
        "SQL Injection": [
            {
                "method": "POST",
                "path": "/login",
                "info": "SQL injection",
                "level": 0,
                "parameter": "user",
                "referer": "",
                "module": "sql",
                "http_request": "POST /login HTTP/1.1",
                "curl_command": "curl -X 'POST' 'http://target/login'",
                "wstg": "x",
            }
        ],
    }
    print("[!] XSS found in GET /search?q=", file=sys.stderr)
    print("[!] SQL Injection found in POST /login (user)", file=sys.stderr)

print("[*] Generating report...", file=sys.stderr)
with open(report_path, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2)
print(f"[+] A report has been generated in the file {report_path}", file=sys.stderr)
