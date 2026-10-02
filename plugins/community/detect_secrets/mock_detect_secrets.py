#!/usr/bin/env python3
"""Mock CLI detect-secrets для контракт-тестов (без пакета detect-secrets).

Имитирует `detect-secrets scan <path> --all-files`: печатает baseline-JSON в
stdout (как настоящий, format_for_output) и прогресс в stderr. Значений
секретов нет — только типы, имена файлов и номера строк.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout),
MOCK_BAD_REPORT=1 (битый JSON), MOCK_FAIL=1 (ненулевой код выхода),
MOCK_EMPTY_RESULTS=1 (baseline с пустым results).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.exit(2)

args = sys.argv[1:]
scan_path = ""
for i, a in enumerate(args):
    if a == "scan" and i + 1 < len(args):
        scan_path = args[i + 1]
        break

print("[*] Scanning (%s)" % (scan_path or "?"), file=sys.stderr)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{"version": "1.5.0", "results": {oops\n')
    sys.exit(0)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_EMPTY_RESULTS") == "1":
    results = {}
else:
    results = {
        "conf/settings.py": [
            {"type": "Secret Keyword",
             "filename": "conf/settings.py", "line_number": 12,
             "hashed_secret": "0" * 40, "is_verified": False},
            {"type": "Secret Keyword",
             "filename": "conf/settings.py", "line_number": 31,
             "hashed_secret": "0" * 40, "is_verified": False},
        ],
        "deploy/values.yaml": [
            {"type": "AWSKeyDetector",
             "filename": "deploy/values.yaml", "line_number": 4,
             "hashed_secret": "0" * 40, "is_verified": False},
        ],
    }

baseline = {
    "version": "1.5.0",
    "plugins_used": [
        {"name": "AWSKeyDetector"},
        {"name": "ArtifactoryDetector"},
        {"name": "Secret Keyword", "keyword_exclude": None},
    ],
    "filters_used": [{"path": "filename", "pattern": r"pin[_-]?code"}],
    "results": results,
}
print(json.dumps(baseline, indent=2))
print("[*] Done", file=sys.stderr)