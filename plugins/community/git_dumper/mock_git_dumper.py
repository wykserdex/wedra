#!/usr/bin/env python3
"""Mock CLI git-dumper для контракт-тестов (без пакета и без сети).

Имитирует `git-dumper <url> <dir>`: пишет построчный лог `[-] ...` в stdout
(как printf() донора), ошибки — в stderr, и создаёт каталог дампа. Сеть не
трогает ни байта.

Аргументы: argv[1] = url, argv[2] = каталог дампа.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой лог),
MOCK_FAIL=1 (ненулевой код выхода), MOCK_NOT_FOUND=1 (цель без .git/HEAD),
MOCK_UNREACHABLE=1 (не удалось подключиться).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

url = sys.argv[1] if len(sys.argv) > 1 else "https://target.example"
directory = sys.argv[2] if len(sys.argv) > 2 else "dump"


def printf(fmt, *args, file=sys.stdout):
    if args:
        fmt = fmt % args
    file.write(fmt)
    file.flush()


if os.environ.get("MOCK_UNREACHABLE") == "1":
    # Настоящий git-dumper сетевых сбоев не перехватывает: session.get() в
    # fetch_git() не обёрнут в try/except, requests.ConnectionError уходит
    # необработанным трейсбеком в stderr, процесс падает с кодом 1.
    printf("\nrequests.exceptions.ConnectionError: HTTPConnectionPool("
           "host='target.example', port=443): Max retries exceeded with url: "
           "/.git/HEAD (Caused by NewConnectionError(\"Failed to establish a "
           "new connection\"))\n", file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_NOT_FOUND") == "1":
    printf("[-] Testing %s/.git/HEAD " % url)
    printf("[404]\n")
    printf("[-] %s/.git/HEAD responded with status code 404\n" % url,
           file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_FAIL") == "1":
    printf("[-] %s/.git/HEAD responded with status code 500\n" % url,
           file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

os.makedirs(directory, exist_ok=True)

printf("[-] Testing %s/.git/HEAD " % url)
printf("[200]\n")
printf("[-] Testing %s/.git/ " % url)
printf("[403]\n")
printf("[-] Fetching common files\n")
printf("[-] Fetching %s/.gitignore [200]\n", url)
printf("[-] Already downloaded %s/.gitignore\n", url)
printf("[-] Fetching %s/.git/HEAD [200]\n", url)
printf("[-] Fetching %s/.git/config [200]\n", url)
printf("[-] Finding refs/\n")
printf("[-] Fetching %s/.git/refs/heads/main [200]\n", url)
printf("[-] Finding packs\n")
printf("[-] Finding objects\n")
printf("[-] Fetching objects\n")
printf("[-] Running git checkout .\n")

with open(os.path.join(directory, "HEAD"), "w", encoding="utf-8") as f:
    f.write("ref: refs/heads/main\n")