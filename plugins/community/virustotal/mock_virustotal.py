#!/usr/bin/env python3
"""Mock донора virustotal для контракт-тестов (без сети, без ключа, без SDK).

Имитирует stdout сниппета из main.py: ровно один JSON-объект с
malicious/total/reputation/found либо с error_class. Аргументы — indicator и
kind (file|domain|ip), как и настоящему сниппету.

Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_FAIL=1 (ненулевой код),
MOCK_NO_REPORT=1 (пустой stdout), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_NOT_FOUND=1 (vt.APIError NotFoundError), MOCK_AUTH_ERR=1,
MOCK_RATE_LIMIT=1, MOCK_SERVER_ERR=1, MOCK_NO_KEY=1 (нет VT_API_KEY),
MOCK_CLEAN=1 (чистый образец: всё по нулям).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    print("[!] virustotal: internal error", file=sys.stderr)
    sys.exit(2)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("{не json от virustotal")
    sys.exit(0)

out = {"malicious": 0, "total": 0, "reputation": 0, "found": False,
       "error": "", "error_class": "", "error_type": ""}

if os.environ.get("MOCK_NOT_FOUND") == "1":
    out["error_class"] = "not_found"
    out["error_type"] = "APIError"
    out["error"] = "('NotFoundError', 'File ... not found')"
elif os.environ.get("MOCK_AUTH_ERR") == "1":
    out["error_class"] = "auth"
    out["error_type"] = "APIError"
    out["error"] = "('AuthenticationError', 'WrongCredentials')"
elif os.environ.get("MOCK_RATE_LIMIT") == "1":
    out["error_class"] = "rate_limit"
    out["error_type"] = "APIError"
    out["error"] = "('QuotaExceededError', 'Request quota exceeded')"
elif os.environ.get("MOCK_SERVER_ERR") == "1":
    out["error_class"] = "network"
    out["error_type"] = "APIError"
    out["error"] = "('ServerError', '<html>503</html>')"
elif os.environ.get("MOCK_NO_KEY") == "1":
    out["error_class"] = "missing_key"
    out["error"] = "VT_API_KEY is not set"
elif os.environ.get("MOCK_CLEAN") == "1":
    out["found"] = True
else:
    out.update({"malicious": 61, "total": 70, "reputation": -42,
                "found": True})

print(json.dumps(out))