#!/usr/bin/env python3
"""Mock gospider CLI для контракт-тестов (без сети и без бинаря gospider).

Имитирует gospider v1.1.6: читает -s <url> и -o <папка>, создаёт папку и пишет
в неё файл с именем хоста (точки → подчёркивания), по одной JSON-строке
SpiderOutput на находку (--json). Показывает находки и в stdout, как настоящий.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_EMPTY=1 (пустой отчёт), MOCK_BAD_REPORT=1 (битые строки вместо JSON),
MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time
from urllib.parse import urlsplit

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("gospider: connection refused", file=sys.stderr)
    sys.exit(2)


def flag(argv, *names):
    for i, arg in enumerate(argv):
        for name in names:
            if arg == name and i + 1 < len(argv):
                return argv[i + 1]
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return None


argv = sys.argv[1:]
site = flag(argv, "-s", "--site")
outdir = flag(argv, "-o", "--output")
if not site or not outdir:
    print("No site in list. Please check your site input again", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[url] - [code-0] - " + site, file=sys.stderr)
    sys.exit(0)

os.makedirs(outdir, exist_ok=True)
report = os.path.join(outdir, (urlsplit(site).hostname or "").replace(".", "_"))

if os.environ.get("MOCK_EMPTY") == "1":
    open(report, "w", encoding="utf-8").close()
    print("Done.", file=sys.stderr)
    sys.exit(0)

lines = [
    {"input": site, "source": "body", "type": "url", "output": site,
     "status": 200, "length": 12},
    {"input": site, "source": "body", "type": "javascript",
     "output": site.rstrip("/") + "/static/app.js", "status": 200,
     "length": 40},
    {"input": site, "source": site.rstrip("/") + "/static/app.js",
     "type": "linkfinder", "output": "/api/v1/users", "status": 0,
     "length": 0},
]
if os.environ.get("MOCK_BAD_REPORT") == "1":
    body = "\n".join("[linkfinder] - " + str(l["output"]) for l in lines)
else:
    body = "\n".join(json.dumps(l) for l in lines)
with open(report, "w", encoding="utf-8") as f:
    f.write(body + "\n")

for line in body.splitlines():
    print(line)
print("Done.", file=sys.stderr)