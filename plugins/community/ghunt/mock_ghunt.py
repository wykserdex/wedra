#!/usr/bin/env python3
"""Mock CLI GHunt для контракт-тестов (без сети и без пакета ghunt).

Имитирует `ghunt <mode> <target> --json <файл>`: пишет выдуманный JSON в файл,
путь которого передан флагом --json (относительно CWD дочернего процесса).
Никаких настоящих токенов и cookies: значение GHUNT_TOKEN в моке не читается.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_EMPTY=1 (пустая выгрузка),
MOCK_NO_REPORT=1 (файл не создан), MOCK_FAIL=1 (ненулевой код),
MOCK_BAD_REPORT=1 (битый JSON).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
mode = args[0] if args else "email"
target = args[1] if len(args) > 1 else ""
report_path = ""
for i, a in enumerate(args):
    if a == "--json" and i + 1 < len(args):
        report_path = args[i + 1]
        break

print(f"[*] GHunt {mode} module — mock run", file=sys.stderr)

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] Not logged in. Run 'ghunt login' first.", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1" or not report_path:
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(report_path, "w", encoding="utf-8") as f:
        f.write('{"gaia_id": 1125936859836,}')
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    payload = {}
else:
    payload = {
        "email": target,
        "gaia_id": 112593685983678043110,
        "name": "Mock Example",
        "picture": "https://lh3.googleusercontent.com/mock/avatar",
        "last_profile_edit": "2024-01-02T03:04:05Z",
        "services": ["YouTube", "Maps", "Photos"],
        "possible_usernames": [],
    }

with open(report_path, "w", encoding="utf-8") as f:
    json.dump(payload, f, ensure_ascii=False)
print(f"[*] Results exported to {report_path}", file=sys.stderr)