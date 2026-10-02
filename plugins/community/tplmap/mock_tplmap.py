#!/usr/bin/env python3
"""Mock tplmap CLI для контракт-тестов (без сети и без пакета tplmap).

tplmap не пишет артефактов — весь отчёт идёт в stdout (utils/loggers.py вешает
stream_handler на sys.stdout, префиксы [+] / [-] / [!]). Мок повторяет этот вид
вывода для подтверждённого SSTI и для «параметры неинъецируемы».

Управляющие env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой
stdout — тест no_report), MOCK_FAIL=1 (ненулевой код выхода), MOCK_CLEAN=1
(SSTI не найдена).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
url = ""
for i, arg in enumerate(args):
    if arg in ("-u", "--url") and i + 1 < len(args):
        url = args[i + 1]

if os.environ.get("MOCK_FAIL") == "1":
    print("Traceback (most recent call last):", file=sys.stderr)
    print("requests.exceptions.ConnectionError: target unreachable",
          file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

print("Tplmap 0.5")
print("    Automatic Server-Side Template Injection Detection and "
      "Exploitation Tool")
print("[+] Testing if GET parameter 'name' is injectable")

if os.environ.get("MOCK_CLEAN") == "1":
    print("[!][core.checks] Tested parameters appear to be not injectable.")
    sys.exit(0)

print("[+] Smarty plugin is testing rendering with tag '{*}'")
print("[+] Mako plugin is testing rendering with tag '${*}'")
print("[+] Jinja2 plugin is testing rendering with tag '{{*}}'")
print("[+] Jinja2 plugin has confirmed injection with tag '{{*}}'")
print("[+] Tplmap identified the following injection point:")
print("")
print("  GET parameter: name")
print("  Engine: Jinja2")
print("  Injection: {{*}}")
print("  Context: text")
print("  OS: linux")
print("  Technique: render")
print("  Capabilities:")
print("")
print("   Shell command execution: no")
print("   Bind and reverse shell: no")
print("   File write: no")
print("   File read: no")
print("   Code evaluation: no")
print("[+] Rerun tplmap providing one of the following options:")
sys.exit(0)