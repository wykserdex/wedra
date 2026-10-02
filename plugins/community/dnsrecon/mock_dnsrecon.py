#!/usr/bin/env python3
"""Mock dnsrecon CLI для контракт-тестов (без сети и без пакета dnsrecon).

Имитирует dnsrecon: читает -d <domain> -t <types> [-D <dict>] -j <файл> и
пишет в файл из -j список словарей-записей в формате dnsrecon (SOA: mname/
address, NS: target/address, A: name/address, MX: name/exchange/address,
CNAME: name/target/address, SPF: strings, AXFR: zone_transfer/ns_server).
Ни одного реального DNS-запроса.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (нет -j файла),
MOCK_FAIL=1 (ненулевой код выхода), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_EMPTY=1 (пустой список записей).
"""
import json
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
domain = opt(argv, "-d") or "example.com"
types = (opt(argv, "-t") or "std").split(",")
json_path = opt(argv, "-j") or "dnsrecon_report.json"

print("[*] std: Performing General Enumeration of Domain: " + domain,
      file=sys.stderr)
if "brt" in types:
    print("[*] brt: Brute-forcing subdomains with dictionary", file=sys.stderr)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    print("[*] dnsrecon: unable to resolve nameserver for domain",
          file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_EMPTY") == "1":
    with open(json_path, "w", encoding="utf-8") as f:
        f.write("[]\n")
    print("[*] 0 Records Found", file=sys.stderr)
    sys.exit(0)

records = [
    {"type": "SOA", "mname": "ns1." + domain, "address": "192.0.2.1"},
    {"type": "NS", "target": "ns1." + domain, "address": "192.0.2.1"},
    {"type": "NS", "target": "ns2." + domain, "address": "192.0.2.2"},
    {"type": "A", "name": domain, "address": "192.0.2.10"},
    {"type": "CNAME", "name": "www." + domain, "target": domain,
     "address": "192.0.2.10"},
    {"type": "MX", "name": domain, "exchange": "mail." + domain,
     "address": "192.0.2.20"},
    {"type": "SPF", "strings": "v=spf1 include:_spf.example.net ~all"},
]
if "brt" in types:
    records.append({"type": "A", "name": "dev." + domain,
                    "address": "192.0.2.30"})
    records.append({"type": "A", "name": "api." + domain,
                    "address": "192.0.2.31"})
if "axfr" in types:
    records.append({"type": "info", "zone_transfer": "success",
                    "ns_server": "192.0.2.1"})
    records.append({"type": "A", "name": "ns1." + domain,
                    "address": "192.0.2.1"})

with open(json_path, "w", encoding="utf-8") as f:
    if os.environ.get("MOCK_BAD_REPORT") == "1":
        f.write("{не json\n")
    else:
        f.write(json.dumps(records, ensure_ascii=False, indent=2) + "\n")

print("[+] %d Records Found — отчёт сохранён в %s" % (len(records), json_path),
      file=sys.stderr)