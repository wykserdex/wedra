#!/usr/bin/env python3
"""Mock abuseipdb SDK для контракт-тестов (без сети и без пакета abuseipdb).

Имитирует то, что печатает SNIPPET из main.py: argv[1] — ip, argv[2] — days,
в stdout ровно один JSON {"payload": {...}} формы ответа APIv2 /check.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_BAD_REPORT=1 (битый JSON → bad_report), MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("abuseipdb: simulated SDK failure", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"payload": {"ipAddress": "1.2.3.4", ', file=sys.stdout)
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

ip = sys.argv[1] if len(sys.argv) > 1 else ""
days = sys.argv[2] if len(sys.argv) > 2 else "30"

DATA = {
    "185.220.101.1": {
        "ipAddress": "185.220.101.1",
        "isPublic": True,
        "ipVersion": 4,
        "isWhitelisted": False,
        "abuseConfidenceScore": 100,
        "countryCode": "DE",
        "usageType": "Data Center/Web Hosting/Transit",
        "isp": "Zwiebelfreunde e.V.",
        "domain": "torservers.net",
        "hostnames": [],
        "isTor": True,
        "totalReports": 842,
        "numDistinctUsers": 120,
        "lastReportedAt": "2026-04-11T09:12:44+00:00",
    },
    "8.8.8.8": {
        "ipAddress": "8.8.8.8",
        "isPublic": True,
        "ipVersion": 4,
        "isWhitelisted": True,
        "abuseConfidenceScore": 0,
        "countryCode": "US",
        "usageType": "Content Delivery Network",
        "isp": "Google LLC",
        "domain": "google.com",
        "hostnames": ["dns.google"],
        "isTor": False,
        "totalReports": 0,
        "numDistinctUsers": 0,
        "lastReportedAt": None,
    },
}

payload = DATA.get(ip, {
    "ipAddress": ip,
    "isPublic": True,
    "ipVersion": 4,
    "isWhitelisted": False,
    "abuseConfidenceScore": 0,
    "countryCode": "",
    "usageType": "",
    "isp": "",
    "domain": "",
    "hostnames": [],
    "isTor": False,
    "totalReports": 0,
    "numDistinctUsers": 0,
    "lastReportedAt": None,
})

print(f"[+] abuseipdb check {ip} за {days} дней: "
      f"score={payload['abuseConfidenceScore']}, "
      f"reports={payload['totalReports']}", file=sys.stderr)
print(json.dumps({"payload": payload}))