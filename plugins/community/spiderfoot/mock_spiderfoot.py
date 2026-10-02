#!/usr/bin/env python3
"""Mock CLI SpiderFoot (sf.py) для контракт-тестов (без сети и без пакета).

Имитирует `sf.py -s <target> -o json -q [-m <module>]`: печатает в stdout
JSON-массив событий по одному (как sfp__stor_stdout при -o json: сначала "[",
элементы через запятую, в конце "]"), прогресс — в stderr. Данные выдуманные.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_EMPTY=1 (скан без событий),
MOCK_NO_REPORT=1 (нет JSON в stdout), MOCK_FAIL=1 (ненулевой код),
MOCK_BAD_REPORT=1 (битый JSON).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[-] Scan failed: invalid target", file=sys.stderr)
    sys.exit(1)

args = sys.argv[1:]
target = ""
for i, a in enumerate(args):
    if a == "-s" and i + 1 < len(args):
        target = args[i + 1]
        break
if not target and args:
    target = args[-1]

if os.environ.get("MOCK_NO_REPORT") == "1":
    print(f"Modules enabled ({2}): sfp__stor_db,sfp__stor_stdout", file=sys.stderr)
    sys.exit(0)

sys.stdout.write("[")
if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{"generated": 1700000000, "data": }')
    sys.stdout.write("]\n")
    sys.exit(0)

events = []
if os.environ.get("MOCK_EMPTY") != "1":
    events = [
        {"generated": 1700000000, "type": "Internet Name",
         "data": target, "module": "sfp_dns", "source": ""},
        {"generated": 1700000001, "type": "IP Address",
         "data": "203.0.113.7", "module": "sfp_dnsresolve",
         "source": target},
    ]

for i, event in enumerate(events):
    if i:
        sys.stdout.write(",")
    sys.stdout.write(json.dumps(event, ensure_ascii=False))
sys.stdout.write("]\n")
print(f"Scan completed, {len(events)} elements", file=sys.stderr)