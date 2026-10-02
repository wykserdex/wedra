#!/usr/bin/env python3
"""Mock CLI you-get для контракт-тестов (без сети и без пакета you-get).

Имитирует --json: печатает в stdout JSON-объект в формате
you_get/json_output.py (url, title, site, streams, audiolang/extra), с
indent=4 и ensure_ascii=False. Как и настоящий you-get, требует в argv --json
(иначе usage в stderr и ненулевой exit: -i/-u/--json — взаимоисключающая
группа dry-run), флагов «скачать» не требует. Режимы env: MOCK_SLEEP=N
(wall_timeout), MOCK_NO_REPORT=1 (пустой stdout, тест no_report), MOCK_FAIL=1
(тест tool_failed), MOCK_FAIL_RETRY=1 (ошибка 429, tool_failed c retryable),
MOCK_BAD_REPORT=1 (битый JSON), MOCK_MINIMAL=1 (только url и title),
MOCK_MULTI=1 (два JSON-объекта подряд — берётся последний).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
url = next((a for a in args if not a.startswith("-")), "")

if os.environ.get("MOCK_FAIL_RETRY") == "1":
    print("you-get: HTTP Error 429: Too Many Requests", file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_FAIL") == "1":
    print(f"you-get: unsupported url {url}", file=sys.stderr)
    sys.exit(1)

if not url:
    print("usage: you-get [OPTION]... [URL]...", file=sys.stderr)
    sys.exit(2)
if "--json" not in args:
    print("usage: you-get [--info|--url|--json] [URL]...", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_MINIMAL") == "1":
    report = {"url": url, "title": "Demo Clip"}
elif os.environ.get("MOCK_MULTI") == "1":
    report = {"url": url, "title": "Demo Clip (part 1)", "site": "DemoSite",
              "streams": {"__default__": {"container": "mp4",
                                          "src": ["https://example.com/a"]}}}
    sys.stdout.write(json.dumps(report, indent=4, ensure_ascii=False) + "\n")
    report = {"url": url, "title": "Demo Clip (part 2)", "site": "DemoSite",
              "streams": {"__default__": {"container": "mp4",
                                          "src": ["https://example.com/b"]},
                          "dash-1080": {"container": "mp4",
                                        "src": ["https://example.com/c"]}}}
else:
    report = {
        "url": url,
        "title": "Demo Clip",
        "site": "DemoSite",
        "streams": {
            "low": {"container": "mp4", "size": 564215,
                    "src": ["https://example.com/low.mp4"]},
            "medium": {"container": "mp4", "size": 1200000,
                       "src": ["https://example.com/medium.mp4"]},
            "dash-1080": {"container": "mp4", "size": 4200000,
                          "src": ["https://example.com/high.mp4"]},
        },
        "audiolang": "en-US",
        "extra": {"referer": "https://example.com/"},
    }

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{\n    "url": "https://example.com",\n'
                     '    "title": "Demo Clip",\n')
    sys.exit(0)

sys.stdout.write(json.dumps(report, indent=4, ensure_ascii=False) + "\n")
print("[info] site: DemoSite", file=sys.stderr)
