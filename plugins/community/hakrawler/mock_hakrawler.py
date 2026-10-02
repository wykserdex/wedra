#!/usr/bin/env python3
"""Mock hakrawler CLI для контракт-тестов (без сети и без бинаря hakrawler).

Имитирует hakrawler v2: URL берёт построчно из stdin, печатает в stdout по
JSON-объекту Result {"Source","URL","Where"} на находку (режим -json),
предупреждения — в stderr. Если stdin опустел, выводит «No urls detected» и
выходит с кодом 1 — как настоящий.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_EMPTY=1 (цель ничего не
отдала), MOCK_BAD_REPORT=1 (мусор вместо JSON), MOCK_PLAIN=1 (режим без
-json: голые ссылки), MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("hakrawler: dial tcp: connection refused", file=sys.stderr)
    sys.exit(1)

targets = [line.strip() for line in sys.stdin.read().splitlines()
           if line.strip()]
if not targets:
    print("No urls detected. Hint: cat urls.txt | hakrawler", file=sys.stderr)
    sys.exit(1)

target = targets[0].rstrip("/")
finds = [
    {"Source": "href", "URL": target + "/login", "Where": ""},
    {"Source": "script", "URL": target + "/static/app.js", "Where": ""},
    {"Source": "form", "URL": target + "/search", "Where": ""},
    {"Source": "href", "URL": target + "/about", "Where": ""},
]

if os.environ.get("MOCK_BAD_REPORT") == "1":
    for item in finds:
        print("[" + item["Source"] + "] " + item["URL"])
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    print("No URLs were found. This usually happens when a domain is "
          "specified (https://example.com), but it redirects to a subdomain "
          "(https://www.example.com).", file=sys.stderr)
    sys.exit(0)

for item in finds:
    if os.environ.get("MOCK_PLAIN") == "1":
        print(item["URL"])
    else:
        print(json.dumps(item))
print("[+] found %d links" % len(finds), file=sys.stderr)