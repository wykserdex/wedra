#!/usr/bin/env python3
"""Mock донора censys для контракт-тестов (без сети, без ключа, без пакета).

Имитирует stdout сниппета из main.py: ровно один JSON-объект с нормализованными
services/location/found либо с error_class. Цель приходит первым аргументом,
как и настоящему сниппету.

Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_FAIL=1 (ненулевой код),
MOCK_NO_REPORT=1 (пустой stdout), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_NOT_FOUND=1 (CensysHostNotFoundException), MOCK_RATE_LIMIT=1,
MOCK_AUTH_ERR=1 (CensysInvalidAPIKeyException), MOCK_NO_KEY=1
(CensysMissingApiKeyException), MOCK_EMPTY=1 (хост без сервисов).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    print("[!] censys: internal error", file=sys.stderr)
    sys.exit(2)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("{не json от censys")
    sys.exit(0)

out = {"services": [], "location": {}, "found": False, "error": "",
       "error_class": "", "error_type": ""}

if os.environ.get("MOCK_NOT_FOUND") == "1":
    out["error_class"] = "not_found"
    out["error_type"] = "CensysHostNotFoundException"
    out["error"] = "Host not found in Censys"
elif os.environ.get("MOCK_RATE_LIMIT") == "1":
    out["error_class"] = "rate_limit"
    out["error_type"] = "CensysRateLimitExceededException"
    out["error"] = "You have exceeded your rate limit"
elif os.environ.get("MOCK_AUTH_ERR") == "1":
    out["error_class"] = "auth"
    out["error_type"] = "CensysInvalidAPIKeyException"
    out["error"] = "Invalid API key"
elif os.environ.get("MOCK_NO_KEY") == "1":
    out["error_class"] = "missing_key"
    out["error_type"] = "CensysMissingApiKeyException"
    out["error"] = "CENSYS_API_ID and CENSYS_API_SECRET are required"
elif os.environ.get("MOCK_EMPTY") == "1":
    out["found"] = True
else:
    out.update({
        "services": [
            {"port": 53, "service_name": "DNS",
             "transport_protocol": "UDP", "observed_at": "2021-02-28T23:58:55Z"},
            {"port": 443, "service_name": "HTTP",
             "transport_protocol": "TCP", "observed_at": "2021-02-28T23:58:55Z"},
        ],
        "location": {
            "continent": "North America",
            "country": "United States",
            "country_code": "US",
            "postal_code": "",
            "timezone": "America/Chicago",
            "coordinates": {"latitude": 37.751, "longitude": -97.822},
            "registered_country": "United States",
            "registered_country_code": "US",
        },
        "found": True,
    })

print(json.dumps(out))