#!/usr/bin/env python3
"""Mock arjun CLI для контракт-тестов (без сети и без пакета arjun).

Имитирует arjun 2.2.7: читает -u <url> и -oJ <file>, пишет JSON-репорт
{"<url>": {"params": [...], "method": "GET", "headers": {...}}} в указанный файл
и печатает баннер/ход фаззинга в stdout, как настоящий. Режимы env:
MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (нет файла и нет сообщения о
пустом результате — тест no_report), MOCK_EMPTY=1 ("No parameters were
discovered." без файла — пустой результат), MOCK_SKIP=1 (цель пропущена),
MOCK_BAD_REPORT=1 (битый JSON в файле), MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] arjun: connection error", file=sys.stderr)
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
json_file = flag(argv, "-oJ", "-o", "--output-json")
if not url:
    print("[!] No target(s) specified", file=sys.stderr)
    sys.exit(1)

print("""
    _
   /_| _ '
  (  |/ /(//) v2.2.7
      _/
""")
print("[*] Probing the target for stability")

if os.environ.get("MOCK_SKIP") == "1":
    print(f"[!] Skipped {url} due to errors")
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    print("[*] No parameters were discovered.")
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1" or not json_file:
    sys.exit(0)

report = {
    url: {
        "params": ["debug", "admin", "token"],
        "method": "GET",
        "headers": {"User-Agent": "arjun"},
    }
}
with open(json_file, "w", encoding="utf-8") as f:
    if os.environ.get("MOCK_BAD_REPORT") == "1":
        f.write('{"' + url + '": {"params": ["debug",,]}')
    else:
        json.dump(report, f, sort_keys=True, indent=4)

print("[*] Processing chunks: 3/3")
print("[*] Parameters found: 3  " + ", ".join(report[url]["params"]))