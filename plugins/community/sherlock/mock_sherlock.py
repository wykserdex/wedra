#!/usr/bin/env python3
"""Mock CLI sherlock для контракт-тестов (без сети и без пакета sherlock).

Имитирует текстовый репорт sherlock в stdout: заголовок «[*] Checking
username ... on:», строки «[+] Сайт: url» для найденных и «[-] Сайт: Not
Found!» для ненайденных. Аргументы: <username> [--site S ...] --print-all
--no-color --timeout N. Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (тест tool_failed),
MOCK_EMPTY=1 (ник нигде не найден), MOCK_SITES_GONE=1 (все сайты минус).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] Sherlock received an exception while trying to query a site",
          file=sys.stderr)
    sys.exit(2)

argv = sys.argv[1:]
username = ""
sites = []
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--site":
        i += 1
        if i < len(argv):
            sites.append(argv[i])
    elif a.startswith("-"):
        i += 1
        if a in ("--timeout",):
            i += 1
    else:
        username = a
    i += 1

if os.environ.get("MOCK_NO_REPORT") == "1" or not username:
    sys.exit(0)

print(f"[*] Checking username {username} on:")

if os.environ.get("MOCK_EMPTY") == "1":
    for name in (sites or ["GitHub", "GitLab", "Twitter", "Reddit"]):
        print(f"[-] {name}: Not Found!")
    print("[*] Search completed with 0 results")
    sys.exit(0)

if os.environ.get("MOCK_SITES_GONE") == "1":
    for name in (sites or ["GitHub", "GitLab", "Twitter", "Reddit"]):
        print(f"[-] {name}: Not Found!")
    print("[*] Search completed with 0 results")
    sys.exit(0)

for name in (sites or ["GitHub", "GitLab", "Twitter", "Reddit"]):
    if name.lower() == "github":
        print(f"[+] GitHub: https://github.com/{username}")
    elif name.lower() == "gitlab":
        print(f"[+] GitLab: https://gitlab.com/{username}")
    else:
        print(f"[-] {name}: Not Found!")
print(f"[*] Search completed with {len(sites) or 2} results")