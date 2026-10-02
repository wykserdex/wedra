#!/usr/bin/env python3
"""Mock linkfinder.py для контракт-тестов (без сети и без пакета linkfinder).

Имитирует `-i <url> -o cli [-d] [-r <regex>] -t N`: печатает находки в stdout
по одной в строке (как cli_output), при -d печатает служебные строки
"Running against: <url>", разделители "Usage:"/"Error:" — как сам инструмент.
Набор находок зависит от цели (обычная / без находок / deface-хвост) и от
-р фильтра. Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_FAIL=1
(ненулевой код выхода), MOCK_ERROR=1 (ошибка инструмента, но код выхода 0 —
как у настоящего linkfinder), MOCK_BAD_LINKS=1 (мусорные строки в выводе).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
url = ""
regex = ""
crawl = False
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("-i", "-o", "-t", "-r") and i + 1 < len(args):
        value = args[i + 1]
        if arg == "-i":
            url = value
        elif arg == "-r":
            regex = value
        i += 2
        continue
    if arg == "-d":
        crawl = True
        i += 1
        continue
    i += 1

if os.environ.get("MOCK_FAIL") == "1":
    print("mock: искусственная ошибка", file=sys.stderr)
    sys.exit(1)

if not url:
    print("Usage: python mock_linkfinder.py [Options]")
    print("Error: no input")
    sys.exit(0)

if os.environ.get("MOCK_ERROR") == "1":
    print("Usage: python mock_linkfinder.py [Options]")
    print("Error: invalid input defined or SSL error: timed out")
    sys.exit(0)

if crawl:
    print("Running against: %s" % url)
    print("")

links = []
if os.environ.get("MOCK_NO_LINKS") != "1":
    links = [
        "https://api.example.com/v1/users",
        "/api/login",
        "/static/app.js",
        "../admin/panel.php",
        "/uploads/shell.php",
    ]
    if regex:
        import re as _re
        try:
            pattern = _re.compile(regex)
        except _re.error:
            pattern = None
        if pattern is not None:
            links = [link for link in links if pattern.search(link)]

if os.environ.get("MOCK_BAD_LINKS") == "1":
    links = ["   ", "Running against: https://example.com/app.js"] + links

for link in links:
    print(link)

print("[*] linkfinder mock: %d находок из %s" % (len(links), url),
      file=sys.stderr)
