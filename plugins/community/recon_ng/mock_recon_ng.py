#!/usr/bin/env python3
"""Mock CLI recon-ng для контракт-тестов (без сети и без пакета recon-ng).

Имитирует `recon-ng -w <workspace> -r run.rcn --no-*`: читает .rcn-сценарий,
печатает его строки (Framework._script это делает), затем отрабатывает модуль
reporting/json — пишет results.json в текущий каталог, как настоящий (путь
передан относительным именем). Воркспейс не создаётся: он и в жизни лежит в
~/.recon-ng, а тесты туда не лезут. Данные в отчёте выдуманные.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_EMPTY=1 (пустой отчёт),
MOCK_NO_REPORT=1 (файл не создан), MOCK_FAIL=1 (ненулевой код),
MOCK_BAD_REPORT=1 (битый JSON).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
script_name = ""
for i, a in enumerate(args):
    if a == "-r" and i + 1 < len(args):
        script_name = args[i + 1]
        break

print("  [recon-ng v5.1.2, mock]")
print("[*] Version check disabled.")
print("[*] Marketplace disabled.")

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] Module not found: recon/broken/module", file=sys.stderr)
    sys.exit(1)

if not script_name or not os.path.isfile(script_name):
    print(f"[!] Script file '{script_name}' not found.", file=sys.stderr)
    sys.exit(1)

with open(script_name, encoding="utf-8") as f:
    lines = [ln.strip() for ln in f.read().splitlines() if ln.strip()]

target = ""
modules = []
for line in lines:
    print(line)
    if line.startswith("modules load "):
        modules.append(line[len("modules load "):])
    if line.startswith("options set SOURCE "):
        target = line[len("options set SOURCE "):]

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] reporting/json not loaded.")
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open("results.json", "w", encoding="utf-8") as f:
        f.write('{"hosts": [{"host": },]}')
    print("[*] 0 records added to 'results.json'.")
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    report = {"hosts": [], "contacts": [], "credentials": []}
else:
    report = {
        "hosts": [
            {"host": f"www.{target}", "ip_address": "203.0.113.7",
             "region": "", "country": "", "latitude": "", "longitude": "",
             "notes": "", "module": "certificate_transparency"},
            {"host": target, "ip_address": "203.0.113.8",
             "region": "", "country": "", "latitude": "", "longitude": "",
             "notes": "", "module": "resolve"},
        ],
        "contacts": [],
        "credentials": [],
    }

with open("results.json", "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False)
total = sum(len(v) for v in report.values() if isinstance(v, list))
print(f"[*] {total} records added to 'results.json'.")
print("[*] Run finished.")