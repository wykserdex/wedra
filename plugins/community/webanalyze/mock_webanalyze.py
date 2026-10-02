#!/usr/bin/env python3
"""Mock webanalyze для контракт-тестов (без сети и без Go-сборки).

Имитирует `webanalyze -host <host> -output json -silent [-apps F] [-crawl N]
[-search=false]`: читает флаги, печатает в stdout одну строку JSON
{"hostname": ..., "matches": [{"app_name", "version"}]} и баннер в stderr.
Хосты из .invalid отдаются пустым списком совпадений — так проверяется
count=0 без ошибки. Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (пустой stdout, тест no_report), MOCK_FAIL=1 (ненулевой код,
тест tool_failed), MOCK_BAD_REPORT=1 (мусор вместо JSON, тест bad_report).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
host = None
apps = None
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("-host", "-apps", "-crawl"):
        value = args[i + 1] if i + 1 < len(args) else None
        if arg == "-host":
            host = value
        elif arg == "-apps":
            apps = value
        i += 2
        continue
    i += 1

if not host:
    print("webanalyze: no host given", file=sys.stderr)
    sys.exit(1)

print(":: webanalyze : mock", file=sys.stderr)
print(":: apps       : %s" % (apps or "technologies.json"), file=sys.stderr)

if os.environ.get("MOCK_FAIL") == "1":
    print("webanalyze: %s error: no such host" % host, file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("webanalyze: %s error: dial tcp: i/o timeout" % host, file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print(":: webanalyze  : v1.0")
    print('{"hostname": "%s", "matches": [' % host)
    sys.exit(0)

if host.endswith(".invalid"):
    matches = []
else:
    matches = [
        {"app": {"name": "Nginx"}, "app_name": "Nginx", "version": "1.18.0",
         "matches": [["Server", "nginx"]]},
        {"app": {"name": "Hugo"}, "app_name": "Hugo", "version": "0.121.1",
         "matches": [["Meta", "generator", "Hugo"]]},
        {"app": {"name": "Netlify"}, "app_name": "Netlify", "version": "",
         "matches": [["Server", "Netlify"]]},
    ]

print(json.dumps({"hostname": "https://%s" % host, "matches": matches}))
