#!/usr/bin/env python3
"""Mock CLI buckethacker для контракт-тестов (без сети и без пакета).

Имитирует `buckethacker <name>`: читает позиционное имя, печатает в stdout
JSON-массив записей (по умолчанию) либо человекочитаемый текст (MOCK_TEXT=1) и
лог в stderr — как настоящий инструмент. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (пустой stdout → тест no_report), MOCK_FAIL=1
(ненулевой код выхода → тест tool_failed), MOCK_BAD_REPORT=1 (битый JSON →
тест bad_report), MOCK_EMPTY=1 (пустой результат), MOCK_TEXT=1 (текстовый
вывод вместо JSON).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] buckethacker: no such bucket", file=sys.stderr)
    sys.exit(2)

args = [a for a in sys.argv[1:] if not a.startswith("-")]
if not args:
    print("[!] buckethacker: usage: buckethacker <name>", file=sys.stderr)
    sys.exit(1)
name = args[0]

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

print(f"[*] searching storage services for '{name}'", file=sys.stderr)

if os.environ.get("MOCK_EMPTY") == "1":
    print("[]")
    print(f"[+] no open storage found for '{name}'", file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_TEXT") == "1":
    print(f"{name}: s3  403 AccessDenied  (private)")
    print(f"{name}: azure blob  200 OK  (open)")
    print(f"{name}: gcp  404 NoSuchBucket  (not found)")
    print(f"[+] {name} is open on 1 service", file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('[{"provider": "s3", "open": tru')
    sys.exit(0)

records = [
    {"name": name, "provider": "s3", "open": False,
     "services": ["s3"], "status": "403 AccessDenied"},
    {"name": name, "provider": "azure", "open": True,
     "services": ["azure blob", "azure"], "status": "open"},
]
print(json.dumps(records, indent=2))
print(f"[+] {name} is open on azure", file=sys.stderr)