#!/usr/bin/env python3
"""Mock CLI vol (volatility3) для контракт-тестов — без пакета volatility3.

Аргументы повторяют боевой вызов main.py: -f <image> -o <dir> -q -r json
[-c <config>] <plugin>. В stderr печатает баннер и диагностику (как vol), в
stdout — JSON-рендерер: список строк-колонок, как у --renderer json. Плагин
выбирается по имени: banners.Banners даёт баннеры, остальные — процессы.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout,
тест no_report), MOCK_FAIL=1 (ненулевой код, тест tool_failed),
MOCK_BAD_REPORT=1 (битый JSON, тест bad_report).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

argv = sys.argv[1:]
module = argv[-1] if argv else ""

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("Unsatisfied requirement plugins: nt_symbols not found\n")
    sys.stderr.write("No further results will be produced\n")
    sys.exit(1)

print("Volatility 3 Framework 2.26.2", file=sys.stderr)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write("\n[{\"PID\": 4,,}]\n")
    sys.exit(0)

if "Banners" in module:
    rows = [
        {"Banner": "Linux version 5.15.0-71-generic (mock) #71 SMP Tue May 2"},
        {"Banner": "Linux version 4.18.0-425.3.1.el8 (mock) #1 SMP Wed May 3"},
    ]
else:
    rows = [
        {"PID": 4, "PPID": 0, "ImageFileName": "System", "Threads": 129,
         "Handles": 41207, "SessionId": 0, "Wow64": 0},
        {"PID": 664, "PPID": 4, "ImageFileName": "explorer.exe",
         "Threads": 47, "Handles": 2651, "SessionId": 1, "Wow64": 0,
         "Cmdline": "C:\\Windows\\explorer.exe"},
    ]

sys.stdout.write("\n" + json.dumps(rows, indent=2, sort_keys=True) + "\n")