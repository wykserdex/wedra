#!/usr/bin/env python3
"""Mock sublist3r CLI для контракт-тестов (без сети и без пакета Sublist3r).

Имитирует Sublist3r 1.0: читает -d/-t/-o/-n, печатает баннер и прогресс в
stdout, а список найденных поддоменов кладёт в текстовый файл из -o
(по одному хосту в строке — ровно как write_file донора). Режимы env:
MOCK_SLEEP=N — спать N секунд (тест wall_timeout); MOCK_NO_REPORT=1 — файла
не пишет, печатает «Total Unique Subdomains Found: 0»; MOCK_BAD_REPORT=1 —
вместо файла создаёт каталог с тем же именем (артефакт нечитаем);
MOCK_FAIL=1 — ненулевой код выхода.
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("Error: could not resolve host, max retries exceeded\n")
    sys.exit(2)

args = sys.argv[1:]
domain = ""
outfile = ""
i = 0
while i < len(args):
    flag = args[i]
    if flag in ("-d", "--domain") and i + 1 < len(args):
        domain = args[i + 1]
        i += 2
    elif flag in ("-o", "--output") and i + 1 < len(args):
        outfile = args[i + 1]
        i += 2
    elif flag in ("-t", "--threads", "-p", "--ports", "-e", "--engines") \
            and i + 1 < len(args):
        i += 2
    else:
        i += 1

if not domain or not outfile:
    sys.stderr.write("Error: Please enter a valid domain\n")
    sys.exit(1)

print("                    ____        _     _ _     _   _____")
print("[-] Enumerating subdomains now for %s" % domain)
print("[-] verbosity is enabled, will show the subdomains results in realtime")

found = [domain,
         "www." + domain,
         "mail." + domain,
         "dev." + domain]

if os.environ.get("MOCK_NO_REPORT") == "1":
    found = []

print("[-] Total Unique Subdomains Found: %s" % len(found))

if os.environ.get("MOCK_BAD_REPORT") == "1":
    os.makedirs(outfile, exist_ok=True)
    sys.exit(0)

if found:
    print("[-] Saving results to file: %s" % outfile)
    with open(outfile, "w", encoding="utf-8") as f:
        for host in found:
            f.write(host + "\n")

for host in found:
    print(host)