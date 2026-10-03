#!/usr/bin/env python3
"""Mock joomscan CLI для контракт-тестов (без сети и без Perl-инструмента).

Имитирует joomscan 0.0.7: читает -u <url> [--enumerate-components] --timeout N,
печатает ход проверок в stdout и кладёт текстовый репорт
reports/<host>/<host>_report_<дата>_at_<время>.txt в CWD (как core/report.pl:
каталог reports/<host> создаётся НЕРЕКУРСИВНЫМ os.mkdir, поэтому без готового
reports/ отчёта не будет). Строки репорта — реальные сообщения модулей joomscan:
"[+] <проверка>" из dprint и "[++] <детали>" из tprint/fprint; версия печатается
как реальный ver.pl — после tr остаётся "3.9.24". Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (нет репорта — тест no_report), MOCK_CLEAN=1 (цель
Joomla без находок и компонентов), MOCK_NOT_ALIVE=1 ("The target is not alive!",
без репорта), MOCK_BAD_REPORT=1 (репорт в нечитаемой кодировке), MOCK_FAIL=1
(ненулевой код выхода).
"""
import os
import sys
import time
from urllib.parse import urlsplit

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("Can't locate joomscan.pl core modules", file=sys.stderr)
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
want_components = any(a in ("-ec", "--enumerate-components")
                      for a in argv)
if not url:
    print("[+] No target specified!")
    sys.exit(1)

print("Processing %s/ ...\n" % url)

if os.environ.get("MOCK_NOT_ALIVE") == "1":
    print("[++] The target is not alive!\n")
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

clean = os.environ.get("MOCK_CLEAN") == "1"

lines = ["[+] Detecting Joomla Version", "[++] 3.9.24", ""]
lines += ["[+] FireWall Detector",
          "[++] Firewall detected : Sucuri Firewall (Sucuri Cloudproxy)", ""]
if not clean:
    lines += ["[+] Core Joomla Vulnerability",
              "[++] Target Joomla core is not vulnerable", ""]
    lines += ["[+] Checking Debug Mode status",
              "[++] Debug mode Enabled : %s/" % url, ""]
    lines += ["[+] Checking Directory Listing",
              "[++] directory has directory listing : ",
              "%s/administrator/components" % url, ""]
    lines += ["[+] Finding common backup files name",
              "[++] Backup file is found ",
              "Path : %s/backup.zip" % url, ""]
if want_components and not clean:
    lines += ["[+] Enumeration component (com_content)",
              "[++] Name: com_content",
              "Location : %s/components/com_content/" % url, ""]
    lines += ["[++] Name: com_contact",
              "Location : %s/components/com_contact/" % url, ""]
lines += ["[++] components are not found"]

host = urlsplit(url).netloc or url.strip("/")
report_dir = os.path.join("reports", host)
# как core/report.pl: mkdir не рекурсивный — без готового reports/ в CWD отчёта нет
os.mkdir(report_dir)
report = os.path.join(report_dir,
                      "%s_report_2026-01-01_at_00.00.00.txt" % host)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(report, "wb") as f:
        f.write(b"[+] Detecting Joomla Version\n[++] \xff\xfe not utf-8\n")
else:
    with open(report, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")

for line in lines:
    if line.startswith("[+]") or line.startswith("[++]"):
        print(line)
print("\nYour Report : %s/" % report_dir.replace("\\", "/"))