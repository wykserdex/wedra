#!/usr/bin/env python3
"""Mock whois-сниппета для контракт-тестов (без сети и без python-whois).

Имитирует то, что печатает SNIPPET из main.py: читает домен из sys.argv[1] и
печатает в stdout один JSON — распознанные поля WhoisEntry без сырого `text`.
Домены из .invalid отдаются пустым объектом — так проверяется found=false.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout,
тест no_report), MOCK_FAIL=1 (ненулевой код, тест tool_failed),
MOCK_BAD_REPORT=1 (обрезанный JSON, тест bad_report),
MOCK_NO_MODULE=1 (ModuleNotFoundError, тест python_whois_not_installed).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

domain = sys.argv[1] if len(sys.argv) > 1 else ""

if os.environ.get("MOCK_NO_MODULE") == "1":
    print("ModuleNotFoundError: No module named 'whois'", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_FAIL") == "1" or not domain:
    print("whois: connection refused by whois server", file=sys.stderr)
    sys.exit(2 if os.environ.get("MOCK_FAIL") == "1" else 1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"domain_name": "%s", "registrar": ' % domain)
    sys.exit(0)

if domain.endswith(".invalid"):
    print("Domain not found", file=sys.stderr)
    print(json.dumps({}))
    sys.exit(0)

record = {
    "domain_name": domain,
    "registrar": "MarkMonitor Inc.",
    "creation_date": "1995-08-14T04:00:00Z",
    "expiration_date": "2028-08-13T04:00:00Z",
    "updated_date": "2025-08-14T07:01:31Z",
    "status": ["clientDeleteProhibited", "clientTransferProhibited"],
    "name_servers": ["ns1.example.net", "ns2.example.net"],
    "emails": ["abuse@markmonitor.com"],
    "country": "US",
}
print("whois: %s (%d fields)" % (domain, len(record)), file=sys.stderr)
print(json.dumps(record, ensure_ascii=False))
