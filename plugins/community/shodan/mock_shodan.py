#!/usr/bin/env python3
"""Mock донора shodan для контракт-тестов (без сети, без ключа, без пакета).

Имитирует stdout сниппета из main.py: печатает ровно один JSON-объект с
нормализованными полями хоста (ports/hostnames/vulns/org/found) либо с
error_class. Цель приходит первым аргументом, как и настоящему сниппету.

Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_FAIL=1 (ненулевой код),
MOCK_NO_REPORT=1 (пустой stdout), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_NOT_FOUND=1 (Shodan: «No information available for that IP»),
MOCK_AUTH_ERR=1 (отказ по ключу), MOCK_RATE_LIMIT=1 (лимит запросов),
MOCK_NO_KEY=1 (SHODAN_API_KEY не задан), MOCK_EMPTY=1 (хост без данных).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    print("[!] shodan: internal error", file=sys.stderr)
    sys.exit(2)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("{не json от shodan")
    sys.exit(0)

out = {"ports": [], "hostnames": [], "vulns": [], "org": "", "found": False,
       "error": "", "error_class": "", "error_type": ""}

if os.environ.get("MOCK_NOT_FOUND") == "1":
    out["error_class"] = "not_found"
    out["error_type"] = "APIError"
    out["error"] = "No information available for that IP."
elif os.environ.get("MOCK_AUTH_ERR") == "1":
    out["error_class"] = "auth"
    out["error_type"] = "APIError"
    out["error"] = "Invalid API key"
elif os.environ.get("MOCK_RATE_LIMIT") == "1":
    out["error_class"] = "rate_limit"
    out["error_type"] = "APIError"
    out["error"] = "No more queries left for your subscription"
elif os.environ.get("MOCK_NO_KEY") == "1":
    out["error_class"] = "missing_key"
    out["error"] = "SHODAN_API_KEY is not set"
elif os.environ.get("MOCK_EMPTY") == "1":
    out["found"] = True
else:
    out.update({
        "ports": [80, 443],
        "hostnames": ["example.com", "www.example.com"],
        "vulns": ["CVE-2021-41773", "CVE-2021-42013"],
        "org": "EXAMPLE-CORP",
        "found": True,
    })

print(json.dumps(out))