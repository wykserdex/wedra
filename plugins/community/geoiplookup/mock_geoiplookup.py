#!/usr/bin/env python3
"""Mock geoiplookup для контракт-тестов (без базы и без самого geoiplookup).

Имитирует интерфейс MaxMind (apps/geoiplookup.c): `geoiplookup [-d dir|-f file]
[-v] [-i] [-l] <ip>` печатает в stdout строку ответа С ПОДПИСЬЮ БАЗЫ из
GeoIPDBDescription — `GeoIP Country Edition: US, United States`, диагностику —
в stderr. Реальный инструмент без подписи строки не печатает.
MOCK_CITY=1 печатает строку City-базы `GeoIP City Edition, Rev 1: US, CA,
California, Mountain View, 94043, 37.39, -122.08, 807, 0` — так проверяется
ветка разбора region/city. MOCK_REGION=1 — строку Region-базы
`GeoIP Region Edition, Rev 1: US, CA`. MOCK_LABELED=1 — вид `Country:`/
`Region:`/`City:` без подписи базы. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (пустой stdout — адрес не найден, тест
no_report), MOCK_FAIL=1 (ненулевой код, тест tool_failed),
MOCK_BAD_REPORT=1 (мусорный вывод, тест found=false),
MOCK_NOT_FOUND=1 (строка `GeoIP Country Edition: IP Address not found`).
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

if os.environ.get("MOCK_NOT_FOUND") == "1":
    print("GeoIP Country Edition: IP Address not found")
    sys.exit(0)

if os.environ.get("MOCK_CITY") == "1":
    print("GeoIP City Edition, Rev 1: US, CA, California, Mountain View, "
          "94043, 37.39, -122.08, 807, 0")
    sys.exit(0)

if os.environ.get("MOCK_REGION") == "1":
    print("GeoIP Region Edition, Rev 1: US, CA")
    sys.exit(0)

if os.environ.get("MOCK_LABELED") == "1":
    print("Country: United States")
    print("Region: California")
    print("City: Mountain View")
    sys.exit(0)

print("GeoIP Country Edition: US, United States")
