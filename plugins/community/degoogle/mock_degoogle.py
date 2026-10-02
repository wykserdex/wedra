#!/usr/bin/env python3
"""Mock degoogle CLI для контракт-тестов (без сети и без пакета degoogle).

Имитирует degoogle 1.0.x: печатает в stdout текстовый отчёт в точном формате
донора («-- N results --» + блоки «описание, URL»), без файлов в CWD. Домен
берётся из позиционного аргумента query (кавычки снимаются). Режимы env:
MOCK_SLEEP=N — спать N секунд (тест wall_timeout); MOCK_NO_RESULTS=1 —
напечатать «no results»; MOCK_BAD_REPORT=1 — напечатать чужой формат;
MOCK_FAIL=1 — ненулевой код выхода.
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("[!] connection reset by peer\n")
    sys.exit(2)

query = None
for arg in sys.argv[1:]:
    if not arg.startswith("-"):
        query = arg
        break
if not query:
    sys.exit(1)

domain = query.strip().strip('"').split()[0] if query.strip() else "example.org"

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("<html><body>captcha</body></html>")
    sys.exit(0)

if os.environ.get("MOCK_NO_RESULTS") == "1":
    print("no results")
    sys.exit(0)

results = [
    (f"{domain} public contacts and press page",
     f"https://{domain}/contacts"),
    (f"support mailbox info@{domain}, sales line +1 202 555 0143",
     f"https://{domain}/about?ref=42"),
]
final = "-- %i results --\n\n" % len(results)
for desc, url in results:
    final += desc + "\n" + url + "\n\n"
if final[-2:] == "\n\n":
    final = final[:-2]
print(final)
sys.stderr.write(f"[+] degoogle done: {domain}\n")