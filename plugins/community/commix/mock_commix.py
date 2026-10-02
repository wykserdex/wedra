#!/usr/bin/env python3
"""Mock commix CLI для контракт-тестов (без сети и без пакета commix).

Пишет в CWD ровно тот артефакт, который ждёт main.py: JSON-репорт, путь к
которому commix получает через --report-json. Форма схемы повторяет commix
4.2 (target/http_method/command/findings[]/finished/target_os).

Управляющие env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (нет
артефакта), MOCK_BAD_REPORT=1 (битый JSON), MOCK_FAIL=1 (ненулевой код выхода),
MOCK_CLEAN=1 (находок нет — findings пуст).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
report_path = "report.json"
url = ""
parameter = ""
for i, arg in enumerate(args):
    if arg in ("-u", "--url") and i + 1 < len(args):
        url = args[i + 1]
    elif arg.startswith("--report-json="):
        report_path = arg.split("=", 1)[1]
    elif arg == "--report-json" and i + 1 < len(args):
        report_path = args[i + 1]
    elif arg in ("-p", "--test-parameter") and i + 1 < len(args):
        parameter = args[i + 1]

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] commix mock: target unreachable", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] Testing 'addr' parameter with blind payloads", file=sys.stderr)
    sys.exit(0)

findings = []
if url and os.environ.get("MOCK_CLEAN") != "1":
    param = parameter or "addr"
    findings.append({
        "parameter": param,
        "http_method": "GET",
        "technique": "classic command injection",
        "type": "Command injection",
        "boundary": ";",
        "payload": "; sleep 5 #",
        "reproduce": "curl -sS '" + url + "'",
    })
    findings.append({
        "parameter": param,
        "http_method": "GET",
        "technique": "time-based command injection",
        "type": "Command injection",
        "boundary": ";",
        "payload": "; sleep 10 #",
    })

report = {
    "target": url,
    "http_method": "GET",
    "command": "commix " + " ".join(args),
    "started": "2026-01-01 10:00:00",
    "findings": findings,
    "finished": "2026-01-01 10:01:00",
    "requests": 42,
    "target_os": "Unix-like",
    "waf_detected": False,
    "evasion_applied": "",
}

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(report_path, "w", encoding="utf-8") as f:
        f.write('{"target": "' + url + '", "findings": [ {"parameter": ')
    print("[!] commix mock: JSON-репорт битый", file=sys.stderr)
    sys.exit(0)

with open(report_path, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, ensure_ascii=False)
print("[+] Run results stored to the JSON file '" + report_path + "'.",
      file=sys.stderr)