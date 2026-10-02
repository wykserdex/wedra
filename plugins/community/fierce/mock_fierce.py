#!/usr/bin/env python3
"""Mock fierce3 CLI для контракт-тестов (без сети и без пакета fierce3).

Имитирует текстовый вывод fierce: печатает служебные строки NS:/SOA:/Zone:/
Wildcard: и строки находок ровно в donor-виде
`print("Found: {} ({})".format(url, ip))` — имя с точкой на конце. Ни одного
реального DNS-запроса.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_FAIL=1 (ненулевой код
выхода, вывод не-fierce), MOCK_BAD_REPORT=1 (вывод не от fierce: нет ни
'Found:', ни NS:/SOA:), MOCK_EMPTY=1 (маркеры есть, находок нет).
"""
import os
import sys
import time


def opt(argv, flag):
    for i, arg in enumerate(argv):
        if arg == flag and i + 1 < len(argv):
            return argv[i + 1]
        if arg.startswith(flag + "="):
            return arg.split("=", 1)[1]
    return None


if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

argv = sys.argv[1:]
domain = opt(argv, "--domain") or "example.com"
words_path = opt(argv, "--subdomain-file")

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("usage: fierce3 [-h] [--domain DOMAIN]")
    sys.exit(0)

print("NS: ns1.%s. ns2.%s." % (domain, domain))
print("SOA: ns1.%s. (192.0.2.1)" % domain)
print("Zone: failure")

if os.environ.get("MOCK_FAIL") == "1":
    print("Failed to lookup NS/SOA, Domain does not exist")
    sys.exit(255)

print("Wildcard: failure")

if os.environ.get("MOCK_EMPTY") == "1" or not words_path:
    sys.exit(0)

with open(words_path, encoding="utf-8") as f:
    words = [line.strip().lower() for line in f if line.strip()]

IP_POOL = {"mail": "192.0.2.20", "www": "192.0.2.10", "dev": "192.0.2.30",
           "api": "192.0.2.31", "vpn": "192.0.2.40", "old": "192.0.2.50"}
for word in words:
    ip = IP_POOL.get(word)
    if ip is None:
        continue
    print("Found: %s.%s. (%s)" % (word, domain, ip))
    print("Nearby:")
    print("{'neighbour.%s.': 'other.%s.'}" % (domain, domain))
sys.exit(0)