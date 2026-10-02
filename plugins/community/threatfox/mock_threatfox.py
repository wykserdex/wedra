#!/usr/bin/env python3
"""Mock донора ThreatFox для контракт-тестов (без сети).

Имитирует то, что печатает SNIPPET из main.py: argv[1] — indicator, argv[2] —
wall_timeout, в stdout ровно один JSON {"raw": <ответ API>}. Формат ответа —
по документации https://threatfox.abuse.ch/api/ (search_ioc).
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_BAD_REPORT=1 (битый JSON → bad_report), MOCK_FAIL=1 (ненулевой код),
MOCK_HTTP_STATUS=N (ответ сервера с кодом N → tool_failed).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("threatfox: simulated failure", file=sys.stderr)
    sys.exit(2)

http_status = os.environ.get("MOCK_HTTP_STATUS")
if http_status:
    print(json.dumps({"http_status": int(http_status), "error": "http"}))
    sys.exit(1)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"raw": {"query_status": "ok", "data": [', file=sys.stdout)
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

indicator = sys.argv[1] if len(sys.argv) > 1 else ""

HITS = {
    "139.180.203.104": {
        "id": "12",
        "ioc": "139.180.203.104:443",
        "threat_type": "botnet_cc",
        "threat_type_desc": "Indicator that identifies a botnet command&control "
                            "server (C&C)",
        "ioc_type": "ip:port",
        "ioc_type_desc": "ip:port combination that is used for botnet "
                         "Command&control (C&C)",
        "malware": "win.cobalt_strike",
        "malware_printable": "Cobalt Strike",
        "malware_alias": "Agentemis,BEACON,CobaltStrike",
        "malware_malpedia": "https://malpedia.caad.fkie.fraunhofer.de/details/"
                            "win.cobalt_strike",
        "confidence_level": 75,
        "first_seen": "2020-12-06 09:10:23 UTC",
        "last_seen": None,
        "reporter": "abuse_ch",
        "tags": None,
        "malware_samples": [],
    },
}

entry = HITS.get(indicator)
if entry:
    response = {"query_status": "ok", "data": [entry]}
    found = "1 запись"
else:
    response = {"query_status": "no_result", "data": []}
    found = "0 записей"

print(f"[+] ThreatFox search_ioc {indicator}: {found}", file=sys.stderr)
print(json.dumps({"raw": response}))