#!/usr/bin/env python3
"""Mock dirsearch CLI для контракт-тестов (без сети и без пакета dirsearch).

Имитирует `dirsearch -u <url> -o <file> --output-formats json -q ...`: читает
-u и -o, создаёт отчёт в формате JSONReport
({"info": {"args","time"}, "results": [{url,status,contentLength,...}]}).
Отчёт создаётся сразу, даже когда результатов нет, — как у настоящего.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_EMPTY=1 (results: []),
MOCK_NO_REPORT=1 (файл не создан), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("Cannot connect to target: Connection refused", file=sys.stderr)
    sys.exit(1)

argv = sys.argv[1:]


def flag(*names):
    for i, arg in enumerate(argv):
        for name in names:
            if arg == name and i + 1 < len(argv):
                return argv[i + 1]
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return None


target = flag("-u", "--url")
outfile = flag("-o", "--output-file")
if not target or not outfile:
    print("Mandatory option '-u' is missing", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("Task Completed", file=sys.stderr)
    sys.exit(0)

results = []
if os.environ.get("MOCK_EMPTY") != "1":
    base = target.rstrip("/")
    results = [
        {"url": base + "/admin/", "status": 200, "contentLength": 5120,
         "contentType": "text/html", "redirect": ""},
        {"url": base + "/api/config.json", "status": 200,
         "contentLength": 128, "contentType": "application/json",
         "redirect": ""},
        {"url": base + "/backup.zip", "status": 403, "contentLength": 0,
         "contentType": "", "redirect": ""},
    ]

if os.environ.get("MOCK_BAD_REPORT") == "1":
    body = '{"info": {"args": "dirsearch"}, "results": ['
else:
    body = json.dumps({"info": {"args": " ".join(argv),
                                "time": "Mon Jan  1 00:00:00 2024"},
                       "results": results}, indent=4, sort_keys=True)

with open(outfile, "w", encoding="utf-8") as f:
    f.write(body)

print("%d requests completed" % len(results), file=sys.stderr)