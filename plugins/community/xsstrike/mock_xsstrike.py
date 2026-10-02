#!/usr/bin/env python3
"""Mock CLI XSStrike для контракт-тестов (без сети и без пакета xsstrike).

Имитирует `xsstrike -u <url> --skip`: печатает в stdout баннер и строки в формате
XSStrike 3.x с ANSI-цветом (префиксы из core/log.py: [!] info, [+] good,
[~] run, [-] bad). MOCK_OLD=1 — старый формат «Vulnerable webpage:» +
«Vector for <param>:» (режим краула). Режимы env: MOCK_SLEEP=N — спать N секунд
(тест wall_timeout); MOCK_NO_REPORT=1 — не печатать ничего (тест no_report);
MOCK_BAD_REPORT=1 — байты, не декодируемые как UTF-8 (тест bad_report, exit 2);
MOCK_CLEAN=1 — находок нет (ok, vulnerable false);
MOCK_DOM=1 — добавить DOM-XSS находку; MOCK_FAIL=1 — ненулевой код выхода
(тест tool_failed).
"""
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
skip = "--skip" in args

if not url:
    sys.stderr.write("[-] no target supplied\n")
    sys.exit(1)

if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("[-] WAF is dropping suspicious requests.\n")
    sys.exit(1)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.buffer.write(b"[+] Payload: \xff\xfe broken\n")
    sys.stdout.buffer.flush()
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

END = "\033[1;m"
INFO = "\033[1;33m[!]\033[1;m"
GOOD = "\033[1;32m[+]\033[1;m"
RUN = "\033[1;97m[~]\033[1;m"
BAD = "\033[1;31m[-]\033[1;m"

print("XSStrike v3.1.5")
print(RUN + " Checking for DOM vulnerabilities")
if os.environ.get("MOCK_DOM") == "1":
    print(GOOD + " Potentially vulnerable objects found" + END)
    print("<script>document.location.hash</script>")
if os.environ.get("MOCK_CLEAN") == "1":
    print(GOOD + " WAF Status: Offline" + END)
    print(INFO + " Testing parameter: q" + END)
    print(BAD + " No reflection found" + END)
    sys.exit(0)

if os.environ.get("MOCK_OLD") == "1":
    print(GOOD + " Crawling the target" + END)
    print(GOOD + " Vulnerable webpage: https://demo.invalid/reflect" + END)
    print(GOOD + " Vector for q: <hTmL%0dONPOINTeReNTer%0d=%0da=prompt,a()%0dx//"
          + END)
    print(GOOD + " Vulnerable webpage: https://demo.invalid/reflect" + END)
    print(GOOD + " Vector for name: <A%09OnMOUSEoVEr%0a=%0a(prompt)``%0dx//v3dm0s"
          + END)
    sys.exit(0)

print(GOOD + " WAF Status: Offline" + END)
print(INFO + " Testing parameter: q" + END)
print(RUN + " Analysing reflections")
print(INFO + " Reflections found: 1" + END)
print(RUN + " Generating payloads")
print(INFO + " Payloads generated: 2" + END)
print(GOOD + " Payload: </tiTLe><a%0aONMousEOvER%0d=%0d[8].find(confirm)//v3dm0s"
      + END)
print(INFO + " Efficiency: 100" + END)
print(INFO + " Confidence: 100" + END)
print(GOOD + " Payload: <svg/onload=alert(1)>" + END)
print(INFO + " Efficiency: 95" + END)
print(INFO + " Confidence: 90" + END)
if not skip:
    sys.stderr.write("[?] Would you like to continue scanning? [y/N]\n")
