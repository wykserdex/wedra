#!/usr/bin/env python3
"""Mock Corsy CLI для контракт-тестов (без сети и без пакета Corsy).

Corsy печатает человекочитаемый отчёт в stdout и, только когда нашёл хоть одну
мисконфигурацию, пишет JSON-словарь в файл из -o (core/utils.format_result:
{url: {class, description, severity, exploitation, "acao header",
"acac header"}}). Мок повторяет обе стороны.

Управляющие env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (файла нет —
Corsy так и сообщает «No misconfigurations found»), MOCK_EMPTY_REPORT=1 (файл
создан, но пустой — тест no_report), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
url = ""
report_path = ""
for i, arg in enumerate(args):
    if arg in ("-u", "--url") and i + 1 < len(args):
        url = args[i + 1]
    elif arg in ("-o", "--json") and i + 1 < len(args):
        report_path = args[i + 1]

if os.environ.get("MOCK_FAIL") == "1":
    print("Traceback (most recent call last):", file=sys.stderr)
    print("requests.exceptions.ConnectionError: unable to connect",
          file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1" or not report_path:
    print(" No misconfigurations found.")
    sys.exit(0)

root = url.split("//", 1)[-1].split("/")[0]

print(" http://%s  -origin reflected- Class: origin reflected" % root)
print("   -ACAO Header: https://example.com")
print("   -ACAC Header: True")

report = {
    url: {
        "class": "origin reflected",
        "description": "This host allows any origin to make requests to it.",
        "severity": "high",
        "exploitation": "Make requests from any domain you control.",
        "acao header": "https://example.com",
        "acac header": "True",
    },
}

if os.environ.get("MOCK_EMPTY_REPORT") == "1":
    open(report_path, "w", encoding="utf-8").close()
    print(" no misconfigurations found.")
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(report_path, "w", encoding="utf-8") as f:
        f.write('{"%s": {"class": "origin refl' % root)
    sys.exit(0)

with open(report_path, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=4)
print(" findings stored to " + report_path)