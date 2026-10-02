#!/usr/bin/env python3
"""Mock dnstwist CLI для контракт-тестов (без сети и без пакета dnstwist).

Имитирует dnstwist --format json: читает домен (позиционный аргумент) и
флаги -f/-t/--registered, печатает в stdout JSON-массив перестановок
(формат Format().json() из dnstwist.py) и прогресс в stderr. Режимы env:
MOCK_SLEEP=N — спать N секунд (тест wall_timeout); MOCK_NO_REPORT=1 — ничего
не печатать (нулевая перестановка); MOCK_BAD_REPORT=1 — печатать не-JSON;
MOCK_FAIL=1 — ненулевой код выхода.
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("Error fetching original domain: connection timed out\n")
    sys.exit(1)

args = sys.argv[1:]
domain = None
fmt = "cli"
threads = None
registered = False
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("-f", "--format") and i + 1 < len(args):
        fmt = args[i + 1]
        i += 2
    elif arg in ("-t", "--threads") and i + 1 < len(args):
        threads = args[i + 1]
        i += 2
    elif arg in ("--registered", "-r"):
        registered = True
        i += 1
    elif arg.startswith("-"):
        i += 1
    elif domain is None:
        domain = arg
        i += 1
    else:
        i += 1

if not domain:
    sys.stderr.write("Error: domain is required\n")
    sys.exit(1)

sys.stderr.write(f"started {threads or 20} scanner threads\n")
sys.stderr.write(f"permutations: 100.00% of 4 | found: 3\n")

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("<html>not json</html>")
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

rows = [
    {"fuzzer": "*original", "domain": domain,
     "dns_a": ["203.0.113.10"],
     "dns_ns": ["ns1.example-host.net", "ns2.example-host.net"]},
    {"fuzzer": "addition", "domain": domain + "s"},
    {"fuzzer": "vowel-swap", "domain": domain.replace("o", "0", 1),
     "dns_a": ["198.51.100.7"],
     "dns_ns": ["ns1.example-host.net"]},
    {"fuzzer": "typosquatting", "domain": domain.replace(".com", ".cm"),
     "dns_ns": ["ns1.typosquat-host.net"]},
]
if registered:
    rows = [r for r in rows if r.get("dns_a")]

if fmt != "json":
    sys.stderr.write("mock: ожидался --format json\n")
    sys.exit(1)

print(json.dumps(rows, indent=4, sort_keys=True))