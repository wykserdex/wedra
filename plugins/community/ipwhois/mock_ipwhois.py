#!/usr/bin/env python3
"""Mock ipwhois_cli для контракт-тестов (без сети и без пакета ipwhois).

Имитирует ipwhois 1.3.x: читает --addr/--json/--whois/--timeout и печатает
в stdout один json — ровно то, что печатает настоящий ipwhois_cli с --json.
Форма RDAP: {query, network, objects}; форма legacy whois (при --whois):
{query, nets, referral}. Адреса из 192.0.2.0/24 отдаются без сети — так
проверяется found=false. Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (пустой stdout, тест no_report), MOCK_FAIL=1 (ненулевой код,
тест tool_failed), MOCK_BAD_REPORT=1 (битый JSON в stdout, тест bad_report).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
addr = None
legacy = "--whois" in args
if "--addr" in args:
    i = args.index("--addr")
    if i + 1 < len(args):
        addr = args[i + 1]

if os.environ.get("MOCK_FAIL") == "1" or not addr:
    print("ipwhois_cli: unable to resolve the address", file=sys.stderr)
    sys.exit(2 if os.environ.get("MOCK_FAIL") == "1" else 1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"query": "%s", "network": {' % addr)
    sys.exit(0)

if addr.startswith("192.0.2."):
    print(json.dumps({"query": addr}))
    sys.exit(0)

if legacy:
    record = {
        "query": addr,
        "nets": [
            {"handle": "NET-8-8-8-0-1", "cidr": "8.8.8.0/24",
             "country": "US"},
        ],
        "referral": {"city": "Mountain View", "country": "US"},
    }
    print("Legacy Whois lookup for %s" % addr, file=sys.stderr)
else:
    record = {
        "query": addr,
        "network": {
            "handle": "NET-74-125-0-0-1",
            "cidr": "74.125.0.0/16",
            "country": "US",
        },
        "objects": {},
    }
    print("RDAP lookup for %s" % addr, file=sys.stderr)

print(json.dumps(record))
