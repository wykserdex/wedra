#!/usr/bin/env python3
"""Mock CLI emailfinder для контракт-тестов (без сети и без пакета).

Имитирует `emailfinder -d <домен>`: печатает построчный текстовый отчёт по
поисковикам в stdout и диагностику в stderr. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (тест
tool_failed), MOCK_EMPTY=1 (поисковики отработали, адресов нет),
MOCK_BAD_REPORT=1 (мусорный/обрезанный вывод вместо отчёта), MOCK_NO_KEY=1
(инструмент без ключа Hunter: ошибка, упоминающая HUNTER_API_KEY).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[-] emailfinder: search engine returned 503", file=sys.stderr)
    sys.exit(2)

argv = sys.argv[1:]
domain = ""
i = 0
while i < len(argv):
    if argv[i] == "-d":
        i += 1
        if i < len(argv):
            domain = argv[i]
    elif argv[i].startswith("-"):
        pass
    else:
        domain = argv[i]
    i += 1

if os.environ.get("MOCK_NO_REPORT") == "1" or not domain:
    sys.exit(0)

if os.environ.get("MOCK_NO_KEY") == "1":
    print("[-] Hunter.io: missing HUNTER_API_KEY, nothing to search",
          file=sys.stderr)
    sys.exit(3)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("Traceback (most recent call last):")
    print('  File "emailfinder/extractor.py", line 88, in get_emails_from_')
    print("{\"data\": {\"emails\": [")
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    print("[*] Searching from google")
    print("[*] Searching from bing")
    print("[*] Searching from baidu")
    print("[*] No emails found")
    sys.exit(0)

print(f"[*] Searching emails for {domain}")
print("[*] Searching from google")
print(f"[+] info@{domain}")
print(f"[+] admin@{domain}")
print(f"[+] noreply@other-domain.example")
print("[*] Searching from bing")
print(f"[+] admin@{domain}")
print(f"[+] sales@{domain}")
print("[*] Done: 5 hits", file=sys.stderr)