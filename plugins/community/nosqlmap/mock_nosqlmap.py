#!/usr/bin/env python3
"""Mock nosqlmap CLI для контракт-тестов (без сети и без пакета nosqlmap).

Имитирует nosqlmap 0.7 в режиме web-app атаки: печатает в stdout свой ход
работы («Test N: …», «Successful injection!») и пишет в CWD ровно тот файл,
который задан через --savePath, в формате nsmweb.save_to(): «Vulnerable
URLs:», «Possibly Vulnerable URLs:», «Timing based attacks:».

Управляющие env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (нет
артефакта), MOCK_BAD_REPORT=1 (мусор вместо секций отчёта), MOCK_FAIL=1
(ненулевой код выхода), MOCK_CLEAN=1 (инъекций не найдено).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
victim = ""
webport = "80"
uri = "/"
save_path = "report.txt"
for i, arg in enumerate(args):
    if arg == "--victim" and i + 1 < len(args):
        victim = args[i + 1]
    elif arg == "--webPort" and i + 1 < len(args):
        webport = args[i + 1]
    elif arg == "--uri" and i + 1 < len(args):
        uri = args[i + 1]
    elif arg == "--savePath" and i + 1 < len(args):
        save_path = args[i + 1]

if os.environ.get("MOCK_FAIL") == "1":
    print("SyntaxError: NoSQLMap mock failed", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("Looks like the server didn't respond.  Check your options.")
    sys.exit(0)

base = victim + ":" + webport + uri
print("Web App Attacks (GET)")
print("===============")
print("Checking to see if site at " + base + " is up...")
print("App is up!")
print("Using abcd for injection testing.")
print("")
print("URI : " + base + "&acctid[$ne]=abcd")
print("Got response length of 512.")
print("Test 1: PHP/ExpressJS != associative array injection")

clean = os.environ.get("MOCK_CLEAN") == "1"
vuln_url = base.replace("acctid=test", "acctid[$ne]=abcd")

if clean:
    print("No change in response size injecting a random parameter..")
    report = ["Vulnerable URLs:", "",
              "Possibly Vulnerable URLs:", "",
              "Timing based attacks:",
              "String Attack-Unsuccessful", "",
              "Integer attack-Unsuccessful", ""]
else:
    print("Successful injection!")
    print("Test 2: $where injection (string escape)")
    print("Possible injection.")
    print("")
    print("Vulnerable URLs:")
    print(vuln_url)
    print("")
    print("Possibly vulnerable URLs:")
    print("")
    print("Timing based attacks:")
    print("String attack-Successful")
    print("Integer attack-Successful")
    report = ["Vulnerable URLs:", vuln_url, "",
              "Possibly Vulnerable URLs:", "",
              "Timing based attacks:",
              "String Attack-Successful", "",
              "Integer attack-Successful", ""]

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(save_path, "w", encoding="utf-8") as f:
        f.write("<?> not a nosqlmap report at all\n")
    sys.exit(0)

with open(save_path, "w", encoding="utf-8") as f:
    f.write("\n".join(report) + "\n")
print("Results written to " + save_path)