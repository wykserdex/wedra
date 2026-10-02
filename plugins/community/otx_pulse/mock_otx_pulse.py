#!/usr/bin/env python3
"""Mock донора OTX Pulse для контракт-тестов (без сети).

Имитирует то, что печатает SNIPPET из main.py: argv[1] — indicator, argv[2] —
wall_timeout, в stdout ровно один JSON {"indicator_type": ..., "raw": ...} формы
ответа OTX indicators/<type>/<indicator>/general (секция pulse_info).
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_BAD_REPORT=1 (битый JSON → bad_report), MOCK_FAIL=1 (ненулевой код),
MOCK_HTTP_STATUS=N (ответ сервера с кодом N).
"""
import ipaddress
import json
import os
import re
import sys
import time


def otx_type(value):
    try:
        ipaddress.ip_address(value)
        return "IPv6" if ":" in value else "IPv4"
    except ValueError:
        pass
    if re.fullmatch(r"[0-9a-fA-F]{32}|[0-9a-fA-F]{40}|[0-9a-fA-F]{64}", value):
        return "file"
    if value.lower().startswith(("http://", "https://")):
        return "url"
    return "hostname"

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("otx: simulated failure", file=sys.stderr)
    sys.exit(2)

http_status = os.environ.get("MOCK_HTTP_STATUS")
if http_status:
    print(json.dumps({"indicator_type": otx_type(
        sys.argv[1] if len(sys.argv) > 1 else ""),
        "http_status": int(http_status), "error": "http"}))
    sys.exit(1)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"indicator_type": "hostname", "raw": {"pulse_info": ',
          file=sys.stdout)
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

indicator = sys.argv[1] if len(sys.argv) > 1 else ""
kind = otx_type(indicator)

HITS = {
    "malware.wicar.org": {
        "count": 2,
        "pulses": [
            {
                "id": "69ede4900c0c36d508b00892",
                "name": "URLHaus data - 20-04-2025",
                "description": "автоматическая выгрузка URLHaus",
                "created": "2025-04-20T23:31:45.683000",
                "modified": "2025-05-20T23:03:20.084000",
                "tags": ["urlhaus"],
                "author": {"username": "CyberHunterAutoFeed", "id": "182496"},
                "public": 1,
                "subscriber_count": 1672,
            },
            {
                "id": "69e71ebd702704e87cdd1189",
                "name": "Lab 3",
                "description": "pulse для лабораторных тестов OTX",
                "created": "2026-04-21T06:52:41.748000",
                "modified": "2026-05-21T00:04:31.159000",
                "tags": ["lab", "training"],
                "author": {"username": "Fransisco", "id": "398818"},
                "public": 1,
                "subscriber_count": 2,
            },
        ],
    },
}

hit = HITS.get(indicator)
pulses = hit["pulses"] if hit else []
raw = {
    "indicator": indicator,
    "type": kind,
    "base_indicator": {"id": 107597314, "indicator": indicator, "type": kind,
                       "access_type": "public"},
    "pulse_info": {
        "count": hit["count"] if hit else 0,
        "pulses": pulses,
        "references": [],
        "related": {"alienvault": {"adversary": [], "malware_families": [],
                                   "industries": []},
                    "other": {"adversary": [], "malware_families": [],
                              "industries": []}},
    },
    "reputation": 0 if not hit else 3,
    "validation": [],
    "whois": "",
}

print(f"[+] OTX {kind}/{indicator}: {len(pulses)} пульсов", file=sys.stderr)
print(json.dumps({"indicator_type": kind, "raw": raw}))