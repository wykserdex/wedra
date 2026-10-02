#!/usr/bin/env python3
"""Mock geoiplookup для контракт-тестов (без базы и без самого geoiplookup).

Имитирует документированный интерфейс MaxMind: `geoiplookup [-d dir|-f file] <ip>`
печатает в stdout одну строку вида `US, United States`, диагностику — в stderr.
MOCK_CITY=1 печатает помеченные строки `Country:`/`Region:`/`City:` — так
проверяется ветка разбора region/city. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (пустой stdout — адрес не найден, тест
no_report), MOCK_FAIL=1 (ненулевой код, тест tool_failed),
MOCK_BAD_REPORT=1 (мусорный вывод, тест found=false).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
db_flag = None
db_path = None
ip = None
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("-f", "-d"):
        db_flag = arg
        db_path = args[i + 1] if i + 1 < len(args) else None
        i += 2
        continue
    if arg.startswith("-"):
        i += 1
        continue
    ip = arg
    i += 1

if not ip or not db_path:
    print("geoiplookup: no ip or no database given", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_FAIL") == "1":
    print(f"geoiplookup: unable to open {db_path}", file=sys.stderr)
    sys.exit(3)

print(f"Looking up {ip} using {db_flag} {db_path}", file=sys.stderr)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("IP Address not found", file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print("geoiplookup: corrupted database, falling back")
    print("þ not a country line")
    sys.exit(0)

if os.environ.get("MOCK_CITY") == "1":
    print("Country: United States")
    print("Region: California")
    print("City: Mountain View")
    sys.exit(0)

print("US, United States")
