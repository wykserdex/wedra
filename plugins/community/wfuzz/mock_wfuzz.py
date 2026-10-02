#!/usr/bin/env python3
"""Mock CLI wfuzz для контракт-тестов (без сети и без пакета wfuzz).

Имитирует wfuzz 2.1.x: читает -u <url>, -w <wordlist>, --filter <expr>,
-f <путь>,<принтер> и пишет в <путь> JSON-массив объектов (принтер json).
Режимы env: MOCK_SLEEP=N — спать N секунд (тест wall_timeout);
MOCK_NO_REPORT=1 — ничего не писать (тест no_report);
MOCK_BAD_REPORT=1 — записать битый JSON (тест bad_report, exit 2);
MOCK_EMPTY=1 — записать пустой список (нули находок — не ошибка);
MOCK_FAIL=1 — ненулевой код выхода (тест tool_failed).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))


def option_value(argv, names, default=None):
    for i, arg in enumerate(argv):
        if arg in names and i + 1 < len(argv):
            return argv[i + 1]
        for name in names:
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return default


args = sys.argv[1:]
url = option_value(args, ["-u", "--url"])
wordlist = option_value(args, ["-w", "--wordlist"])
matcher = option_value(args, ["--filter"])
raw_out = option_value(args, ["-f"])

if not url:
    sys.stderr.write("[*] Usage: wfuzz [options] -u <url> -w <wordlist>\n")
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print(f"[*] Fuzzing {url}", file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("[*] Fatal error: unable to open wordlist\n")
    sys.exit(2)

if not raw_out:
    sys.stderr.write("[*] Missing -f <file>,<printer>\n")
    sys.exit(1)

out_path = raw_out.rsplit(",", 1)[0] if "," in raw_out else raw_out

if os.environ.get("MOCK_EMPTY") == "1":
    entries = []
else:
    seed = "demo" if "FUZZ" in url else "single"
    entries = [
        {
            "chars": 1256, "code": 200,
            "payload": "admin",
            "lines": 45, "location": "", "method": "GET",
            "post_data": [], "server": "nginx",
            "url": url.replace("FUZZ", "admin"), "words": 120,
        },
        {
            "chars": 319, "code": 301,
            "payload": "backup",
            "lines": 9, "location": "/backup/", "method": "GET",
            "post_data": [], "server": "nginx",
            "url": url.replace("FUZZ", "backup"), "words": 28,
        },
        {
            "chars": 0, "code": 404,
            "payload": "nothing-here",
            "lines": 0, "location": "", "method": "GET",
            "post_data": [], "server": "nginx",
            "url": url.replace("FUZZ", "nothing-here"), "words": 0,
        },
    ]
    print(f"[*] Fuzzing {url} with {seed}", file=sys.stderr)

print(f"Target: {url}", file=sys.stderr)
print("Payload type: file," + str(wordlist), file=sys.stderr)
if matcher:
    print("Filter: " + matcher, file=sys.stderr)
print("Total requests: 3", file=sys.stderr)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(out_path, "w", encoding="utf-8") as f:
        f.write('[{"code": 200, "url": ')
    sys.stderr.write("[*] Report written\n")
    sys.exit(0)

with open(out_path, "w", encoding="utf-8") as f:
    json.dump(entries, f)
sys.stderr.write(f"[*] Results saved in {out_path}\n")
