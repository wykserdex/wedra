#!/usr/bin/env python3
"""Mock CLI h8mail для контракт-тестов (без сети и без пакета h8mail).

Имитирует `h8mail -t <target> -c <cfg> -o <csv> -j <json>`: читает конфиг
(значения ключей игнорирует — тесты идут без ключей), пишет CSV-репорт с
колонками Target,Type,Data и человекочитаемый вывод в stderr. Режимы env:
MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_FAIL=1 (тест tool_failed), MOCK_EMPTY=1 (отчёт без находок),
MOCK_BAD_REPORT=1 (CSV с чужой шапкой), MOCK_JSON_ONLY=1 (только -j, без CSV),
MOCK_STRICT_CONFIG=1 (конфиг обязан быть ini с полным набором ключей [h8mail]).
"""
import configparser
import json
import os
import sys
import time

EXPECTED_KEYS = (
    "hibp", "hunterio", "snusbase_token", "weleakinfo_priv", "weleakinfo_pub",
    "leak-lookup_pub", "leak-lookup_priv", "emailrep", "dehashed_email",
    "dehashed_key", "intelx_key", "intelx_maxfile", "breachdirectory_user",
    "breachdirectory_pass",
)

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] h8mail: target must be an email address", file=sys.stderr)
    sys.exit(2)

argv = sys.argv[1:]
target = ""
config = ""
csv_out = ""
json_out = ""
i = 0
while i < len(argv):
    a = argv[i]
    if a in ("-t", "-c", "-o", "-j") and i + 1 < len(argv):
        value = argv[i + 1]
        if a == "-t":
            target = value
        elif a == "-c":
            config = value
        elif a == "-o":
            csv_out = value
        else:
            json_out = value
        i += 2
        continue
    i += 1

if os.environ.get("MOCK_NO_REPORT") == "1" or not target:
    sys.exit(0)

if not config or not os.path.exists(config):
    print("[!] h8mail: no configuration file given (-c)", file=sys.stderr)
    sys.exit(4)

keys = []
with open(config, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("[") or line.startswith(";"):
            continue
        keys.append(line.split("=", 1)[0].strip())
print("[*] Found %d configuration keys" % len(keys), file=sys.stderr)

if os.environ.get("MOCK_STRICT_CONFIG") == "1":
    parsed = configparser.ConfigParser()
    with open(config, encoding="utf-8") as f:
        parsed.read_file(f)
    section = parsed["h8mail"]
    missing = [k for k in EXPECTED_KEYS if k not in section]
    if missing:
        print("[!] h8mail: config misses keys %s" % ",".join(missing),
              file=sys.stderr)
        sys.exit(5)

print(f"[>] Showing results for {target}", file=sys.stderr)

rows = [
    ("EMAILREP_LEAKS", "103 leaked credentials"),
    ("EMAILREP_SOCIAL", "Twitter"),
    ("HIBP3", "Adobe"),
    ("SCYLLA_SOURCE", "exploit.in"),
    ("SCYLLA_PASSWORD", "sample********"),
]

if os.environ.get("MOCK_EMPTY") == "1":
    rows = []

if os.environ.get("MOCK_JSON_ONLY") == "1":
    if json_out:
        with open(json_out, "w", encoding="utf-8") as f:
            json.dump({target: {"pwned": len(rows), "data": [list(r) for r in rows]}},
                      f, indent=2)
else:
    if os.environ.get("MOCK_BAD_REPORT") == "1":
        header = "email,source,note"
    else:
        header = "Target,Type,Data"
    with open(csv_out, "w", encoding="utf-8", newline="") as f:
        f.write(header + "\n")
        for kind, value in rows:
            f.write(f"{target},{kind},{value}\n")
    if json_out and os.environ.get("MOCK_BAD_REPORT") != "1":
        with open(json_out, "w", encoding="utf-8") as f:
            json.dump({target: {"pwned": len(rows), "data": [list(r) for r in rows]}},
                      f, indent=2)

for kind, value in rows:
    print(f"{kind:<17}|{target} > {value}", file=sys.stderr)
print(f"[>] {target} Breach Found ({len(rows)} elements)", file=sys.stderr)