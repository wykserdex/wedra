#!/usr/bin/env python3
"""Mock cloud_enum CLI для контракт-тестов (без сети и без пакета cloud_enum).

Имитирует cloud_enum 0.8 (initstring/cloud_enum): читает -k (можно повторять),
-l, -f, -m, -qs; печатает баннер и прогресс в stdout, а в лог -l пишет сначала
заголовок "#### CLOUD_ENUM <дата> ####", затем по строке JSON на находку
{"platform","msg","target","access"} (как enum_tools/utils.py: init_logfile +
fmt_output). Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1
(лог не создан — тест no_report), MOCK_EMPTY=1 (только заголовок), MOCK_BAD_REPORT=1
(битая строка JSON), MOCK_NO_KEYWORD=1 (нет -k: argparse-ошибка), MOCK_BAD_MUT=1
(-m на нечитаемый файл — как настоящий, уходит с пустым логом), MOCK_FAIL=1
(ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("cloud_enum: got an unexpected internal error", file=sys.stderr)
    sys.exit(2)


def flags(argv, *names):
    out = []
    for i, arg in enumerate(argv):
        for name in names:
            if arg == name and i + 1 < len(argv):
                out.append(argv[i + 1])
            elif arg.startswith(name + "="):
                out.append(arg.split("=", 1)[1])
    return out


argv = sys.argv[1:]
keywords = flags(argv, "-k", "--keyword")
logfile = flags(argv, "-l", "--logfile")
fmt = flags(argv, "-f", "--format")
mutations = flags(argv, "-m", "--mutations")

print("""
##########################
        cloud_enum
   github.com/initstring
##########################
""")

if not keywords or os.environ.get("MOCK_NO_KEYWORD") == "1":
    print("cloud_enum: error: the following arguments are required: "
          "-k/--keyword", file=sys.stderr)
    sys.exit(2)

print("Keywords:    " + ", ".join(keywords))
print("Mutations:   " + (mutations[0] if mutations else "enum_tools/fuzz.txt"))
print("Brute-list:  " + (mutations[0] if mutations else "enum_tools/fuzz.txt"))
print("")

if mutations and os.environ.get("MOCK_BAD_MUT") == "1":
    print("[!] Cannot access mutations file: " + mutations[0])
    sys.exit(0)

print("[+] Mutations list imported: 42 items")
print("[+] Mutated results: 126 items")
print("[+] Checking for S3 buckets")

records = []
if os.environ.get("MOCK_EMPTY") != "1":
    records = [
        {"platform": "aws", "msg": "OPEN S3 BUCKET",
         "target": "http://acme-prod.s3.amazonaws.com", "access": "public"},
        {"platform": "aws", "msg": "Protected S3 Bucket",
         "target": "http://acme-backup.s3.amazonaws.com",
         "access": "protected"},
        {"platform": "gcp", "msg": "OPEN GCS BUCKET",
         "target": "http://acme-prod.storage.googleapis.com",
         "access": "public"},
    ]

if logfile and os.environ.get("MOCK_NO_REPORT") != "1":
    with open(logfile[0], "w", encoding="utf-8") as f:
        f.write("\n\n#### CLOUD_ENUM 01/01/2026 00:00:00 ####\n")
        if os.environ.get("MOCK_BAD_REPORT") == "1":
            f.write('{"platform": "aws", "msg": "OPEN S3 BUCKET",,}\n')
        else:
            for record in records:
                f.write(json.dumps(record) + "\n")

if logfile:
    print("[PLUGIN] The result will be recorded in " + logfile[0])

for record in records:
    print("  %s: %s" % (record["msg"], record["target"]))

print("\n[+] All done, happy hacking!\n")