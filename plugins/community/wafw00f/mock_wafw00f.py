#!/usr/bin/env python3
"""Mock wafw00f CLI для контракт-тестов (без сети и без пакета wafw00f).

Имитирует 2.4.x: читает `-o <файл> [-a] -T N --no-colors <url>`, пишет в CWD
JSON-массив записей {url, detected, trigger_url, firewall, manufacturer} под
именем из -o и печатает баннер в stdout, отчёт-путь в stderr. Состав отчёта
зависит от цели: обычный URL — Cloudflare, `no-waf` — не найдено, `generic` —
общая детекция. Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (ненулевой код выхода),
MOCK_BAD_REPORT=1 (битый JSON в файле отчёта).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
out_name = "wafw00f_report.json"
findall = False
url = ""
i = 0
while i < len(args):
    arg = args[i]
    if arg == "-o" and i + 1 < len(args):
        out_name = args[i + 1]
        i += 2
        continue
    if arg == "-T" and i + 1 < len(args):
        i += 2
        continue
    if arg == "-a":
        findall = True
        i += 1
        continue
    if arg.startswith("-"):
        i += 1
        continue
    url = arg
    i += 1

print("           ~ WAFW00F : v2.4.2 ~")
print("[*] Checking %s" % url)

if os.environ.get("MOCK_FAIL") == "1":
    print("[!] mock: искусственная ошибка", file=sys.stderr)
    sys.exit(2)

if not url:
    print("mock: нет цели", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[~] Number of requests: 2", file=sys.stderr)
    sys.exit(0)


def record(firewall, manufacturer, found, trigger):
    return {"url": url, "detected": found, "trigger_url": trigger,
            "firewall": firewall, "manufacturer": manufacturer}


if "no-waf" in url:
    report = [record("None", "None", False, None)]
elif "generic" in url:
    report = [record("Generic", "Unknown", True, url + "?x=1")]
else:
    report = [record("Cloudflare", "Cloudflare Inc.", True, url + "?id=1")]
    if findall:
        report.append(record("Generic", "Unknown", True, url + "?id=2"))

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(out_name, "w", encoding="utf-8") as f:
        f.write('[{"url": "%s", "detected": tru' % url)
    print("[+] отчёт записан: %s (битый)" % out_name, file=sys.stderr)
    sys.exit(0)

with open(out_name, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)

for item in report:
    if item["detected"]:
        print("[+] The site %s is behind %s WAF." % (url, item["firewall"]))
    else:
        print("[-] No WAF detected on %s" % url)
print("[~] Number of requests: 2", file=sys.stderr)
print("[+] отчёт записан: %s" % out_name, file=sys.stderr)
