#!/usr/bin/env python3
"""Mock pocsuite3 CLI для контракт-тестов (без сети и без пакета pocsuite3).

Имитирует pocsuite3 2.1.0: читает -u <url> --verify --batch -o <file> [-k KEY],
печатает таблицу результатов в stdout и пишет в файл -o JSON Lines — по строке на
успешную находку {"target","poc_name","result","created_time"} (как плагин
file_record). Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1
(нет файла — тест no_report), MOCK_EMPTY=1 (файл создан, находок нет),
MOCK_BAD_REPORT=1 (битая строка JSON вместо записей), MOCK_FAIL=1 (ненулевой код
выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] poc module has bugs, please check and fix it", file=sys.stderr)
    sys.exit(2)


def flag(argv, *names):
    for i, arg in enumerate(argv):
        for name in names:
            if arg == name and i + 1 < len(argv):
                return argv[i + 1]
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return None


argv = sys.argv[1:]
url = flag(argv, "-u", "--url")
report = flag(argv, "-o", "--output")
keyword = flag(argv, "-k")

print("Pocsuite3 - Open-source vulnerability testing framework")
print("[*] starting at 00:00:00\n")

if not url:
    print("[*] No poc specified, try 'pocsuite -h' or 'pocsuite --help' "
          "for more information")
    sys.exit(1)

records = []
if os.environ.get("MOCK_EMPTY") != "1":
    found = [
        ("cms-demo-sqli", "CMS Demo 1.2 - SQL Injection"),
        ("cms-demo-lfi", "CMS Demo 1.2 - Local File Inclusion"),
    ]
    for poc, info in found:
        records.append({
            "target": url,
            "poc_name": poc,
            "result": {"Info": info},
            "created_time": "2026-01-01 00:00:01",
        })

if os.environ.get("MOCK_NO_REPORT") != "1" and report:
    with open(report, "w", encoding="utf-8") as f:
        if os.environ.get("MOCK_BAD_REPORT") == "1":
            f.write('{"target": "%s", "poc_name": }\n' % url)
        else:
            for record in records:
                f.write(json.dumps(record) + "\n")

print("\n+---------------------+------------------+----------+"
      "------------+--------------+")
print("| target-url          | poc-name         | poc-id   |"
      " component  | status       |")
print("+---------------------+------------------+----------+"
      "------------+--------------+")
for record in records:
    print("| %-19s | %-16s | %-8s | %-10s | %-12s |" % (
        record["target"], record["poc_name"], "-", "demo",
        "success"))
print("+---------------------+------------------+----------+"
      "------------+--------------+")
print("\nsuccess : %d / %d\n" % (len(records), len(records)))
if keyword:
    print("[*] keyword filter: %s" % keyword)
print("[*] shutting down at 00:00:09\n")