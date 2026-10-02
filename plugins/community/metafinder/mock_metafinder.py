#!/usr/bin/env python3
"""Mock metafinder CLI для контракт-тестов (без сети и без пакета metafinder).

Имитирует metafinder 1.2: читает -d/-o/-l/-t и флаги -go/-bi/-ba, создаёт
<output>/<domain>/ и кладёт туда metadata_result.txt в формате
utils/file/parser.py, плюс authors.txt/software.txt. В stdout идёт прогресс,
как у настоящего. Режимы env: MOCK_SLEEP=N — спать N секунд (тест
wall_timeout); MOCK_NO_METADATA=1 — «метаданных нет», отчёта не пишет;
MOCK_BAD_REPORT=1 — пишет мусор вместо отчёта; MOCK_FAIL=1 — ненулевой код
выхода.
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("requests: Read timed out\n")
    sys.exit(1)

args = sys.argv[1:]
domain = ""
folder = "results"
limit = 20
engines = []
i = 0
while i < len(args):
    flag = args[i]
    if flag in ("-d", "--domain") and i + 1 < len(args):
        domain = args[i + 1]
        i += 2
    elif flag in ("-o", "--output") and i + 1 < len(args):
        folder = args[i + 1]
        i += 2
    elif flag in ("-l", "--limit") and i + 1 < len(args):
        limit = args[i + 1]
        i += 2
    elif flag in ("-t", "--threads") and i + 1 < len(args):
        i += 2
    elif flag in ("-go", "--google"):
        engines.append("google")
        i += 1
    elif flag in ("-bi", "--bing"):
        engines.append("bing")
        i += 1
    elif flag in ("-ba", "--baidu"):
        engines.append("baidu")
        i += 1
    else:
        i += 1

if not domain:
    sys.exit(2)

print(f"Searching in {engines[0] if engines else 'google'}")
print("Done")
msg = f"Total files to be analyzed: {limit}"
print(msg)
print("-" * len(msg))

if os.environ.get("MOCK_NO_METADATA") == "1":
    print("No metadata found...")
    sys.exit(0)

directory = os.path.join(folder, domain)
os.makedirs(directory, exist_ok=True)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(os.path.join(directory, "metadata_result.txt"), "w",
              encoding="utf-8") as f:
        f.write("Traceback (most recent call last): <файл> не распознан\n")
    print("All metadata results have been saved in file "
          f"{directory}/metadata_result.txt")
    sys.exit(0)

search_engines = ", ".join(engines) if engines else "google"

blocks = [
    ("annual-report.pdf",
     f"https://{domain}/docs/annual-report.pdf",
     200, search_engines,
     [("Author", "Demo Analyst"), ("Producer", "Demo PDF Suite")]),
    ("press-kit.docx",
     f"https://{domain}/files/press-kit.docx",
     200, search_engines,
     None),
]
report = ""
for name, url, code, se, meta in blocks:
    report += "\n" + name + "\n" + "-" * len(name) + "\n"
    report += f"URL: {url}\n"
    report += f"Status code: {code}\n"
    report += f"Search engines: {se}\n"
    if meta:
        for key, value in meta:
            report += f"|_ {key}: {value}\n"
    else:
        report += "|_ No metadata found\n"

with open(os.path.join(directory, "metadata_result.txt"), "w",
          encoding="utf-8") as f:
    f.write(report)
with open(os.path.join(directory, "authors.txt"), "w", encoding="utf-8") as f:
    f.write("Demo Analyst\n")
with open(os.path.join(directory, "software.txt"), "w", encoding="utf-8") as f:
    f.write("Demo PDF Suite\n")

print("Analyzing metadata...")
print(f"All metadata results have been saved in file "
      f"{directory}/metadata_result.txt")