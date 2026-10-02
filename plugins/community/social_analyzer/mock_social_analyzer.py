#!/usr/bin/env python3
"""Mock CLI social-analyzer для контракт-тестов (без сети и без пакета).

Имитирует qeeqbox/social-analyzer: принимает --username <u>, --websites "..." и
--output json, печатает JSON-машинный отчёт в stdout и человекочитаемый ход
работы в stderr. Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (тест tool_failed),
MOCK_EMPTY=1 (профилей нет), MOCK_PLAIN=1 (текстовый вывод вместо JSON —
проверка fallback-разбора), MOCK_BAD_JSON=1 (нечитаемый JSON в stdout).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[-] social-analyzer: request failed for all websites",
          file=sys.stderr)
    sys.exit(2)

argv = sys.argv[1:]
username = ""
websites = []
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--username":
        i += 1
        if i < len(argv):
            username = argv[i]
    elif a == "--websites":
        i += 1
        if i < len(argv):
            websites = argv[i].split()
    elif a.startswith("-"):
        pass
    i += 1

if os.environ.get("MOCK_NO_REPORT") == "1" or not username:
    sys.exit(0)

print(f"[*] Checking username: {username} on "
      f"{len(websites) if websites else 400} websites", file=sys.stderr)

if os.environ.get("MOCK_PLAIN") == "1":
    print(f"[*] Checking username: {username}")
    for name in websites or ["github", "reddit"]:
        print(f"[*] Found: https://{name}.com/user/{username}")
    for name in (websites or ["github", "reddit"])[:1]:
        print(f"[*] Not found: https://twitter.com/{username}")
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    print(json.dumps({"detected_profiles": [], "unknown_profiles": [],
                      "failed_profiles": []}))
    sys.exit(0)

names = websites or ["github", "reddit"]
report = {
    "detected_profiles": [
        {"name": name.title(), "link": f"https://{name}.com/user/{username}",
         "rate": 100, "status": "detected"} for name in names
    ],
    "unknown_profiles": [
        {"name": "Twitter", "link": f"https://twitter.com/{username}",
         "rate": 0, "status": "unknown"}
    ],
    "failed_profiles": [
        {"name": "Instagram", "link": f"https://instagram.com/{username}",
         "rate": 0, "status": "failed"}
    ],
}

if os.environ.get("MOCK_BAD_JSON") == "1":
    sys.stdout.write('{"detected_profiles": [{"name": "GitHub", "link": ')
    sys.exit(0)

print(json.dumps(report, indent=2))
print(f"[*] Done: {len(report['detected_profiles'])} profiles found",
      file=sys.stderr)