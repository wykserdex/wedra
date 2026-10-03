#!/usr/bin/env python3
"""Mock CLI ghauri для контракт-тестов (без сети и без пакета ghauri).

Имитирует РЕАЛЬНОЕ поведение ghauri 1.4.3, а не то, что хочется видеть:

* весь ColoredLogger идёт в stderr (StreamHandler без stream → sys.stderr),
  там же баннер и строки "[HH:MM:SS] [LEVEL] сообщение";
* ~/.ghauri/<netloc>/log (FileHandler с "%(message)s") имеет
  handler.setLevel(SUCCESS), поэтому туда попадают ТОЛЬКО записи уровня
  SUCCESS — блок "Parameter:/Type:/Title:", а NOTICE-строка "appears to be
  ... injectable" и CRITICAL-итоги в файл НЕ пишутся (проверено на 1.4.3:
  на неинъекционном цели файл 0 байт);
* ANSI-последовательности приезжают внутри самого сообщения (mc/nc из
  ghauri/common/colors.py), поэтому в stderr они есть.

Цель подменяет HOME/USERPROFILE, cwd = временный каталог (main.py так и делает),
поэтому лог ищется относительно CWD.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_FAIL=1 (ненулевой код),
MOCK_NO_REPORT=1 (нет ни лога, ни вывода), MOCK_NO_PARAMS=1 (в URL нет
параметров), MOCK_NOT_VULNERABLE=1 (параметры не инъекционны).
"""
import os
import sys
import time
from urllib.parse import urlparse

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.exit(2)

MC = "\x1b[0m\x1b[2m\x1b[37m"
NC = "\x1b[0m\x1b[22m\x1b[39m"

args = sys.argv[1:]
url = args[args.index("-u") + 1] if "-u" in args else "http://example.com/"

netloc = urlparse(url).netloc.split(":")[0]

stream = []
log = []


def notice(message):
    stream.append(f"[12:00:00] [INFO] {message}")


def critical(message):
    stream.append(f"[12:00:03] [CRITICAL] {message}")


def success(message):
    stream.append(message)
    log.append(message)


if os.environ.get("MOCK_NO_PARAMS") == "1":
    stream.append("\n[*] starting @ 12:00:00 /2026-01-01/")
    critical("no parameter(s) found for testing in the provided data "
             "(e.g. GET parameter 'id' in 'www.site.com/index.php?id=1')")
    stream.append("\n[*] ending @ 12:00:01 /2026-01-01/")
elif os.environ.get("MOCK_NOT_VULNERABLE") == "1":
    stream.append("\n[*] starting @ 12:00:00 /2026-01-01/")
    notice("testing for SQL injection on GET parameter 'id'")
    stream.append("[12:00:01] [WARNING] heuristic (basic) test shows that GET "
                  f"parameter '{MC}id{NC}' might not be injectable")
    stream.append("[12:00:02] [WARNING] GET parameter "
                  f"'{MC}id{NC}' does not seem to be injectable")
    critical("all tested parameters do not appear to be injectable.")
    stream.append("\n[*] ending @ 12:00:03 /2026-01-01/")
else:
    stream.append("\n[*] starting @ 12:00:00 /2026-01-01/")
    notice("testing for SQL injection on GET parameter 'id'")
    stream.append("[12:00:01] [WARNING] heuristic (basic) test shows that GET "
                  f"parameter '{MC}id{NC}' might be injectable "
                  "(possible DBMS: 'MySQL')")
    notice(f"GET parameter '{MC}id{NC}' appears to be "
           f"'{MC}OR boolean-based blind - WHERE or HAVING clause{NC}' "
           "injectable")
    notice(f"POST parameter '{MC}name{NC}' appears to be "
           f"'{MC}MySQL inline query (QUOTE){NC}' injectable")
    stream.append("\n[*] ending @ 12:00:07 /2026-01-01/")
    success("Ghauri identified the following injection point(s) with a total "
            "of 42 HTTP(s) requests:\n---\n"
            f"Parameter: id (GET)\n"
            "    Type: boolean-based\n"
            "    Title: OR boolean-based blind - WHERE or HAVING clause\n"
            "    Payload: id=1 OR 6162=6162\n"
            "Parameter: name (POST)\n"
            "    Type: inline query\n"
            "    Title: MySQL inline query (QUOTE)\n"
            "    Payload: name=1' or '1'='1\n"
            "---")

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.stderr.write("[*] log not written\n")
    sys.exit(0)

path = os.path.join(".ghauri", netloc, "log")
os.makedirs(os.path.dirname(path), exist_ok=True)
with open(path, "w", encoding="utf-8") as f:
    f.write(("\n".join(log) + "\n") if log else "")
sys.stderr.write("\n".join(stream) + "\n")
sys.stderr.write(f"[*] fetched data logged to text files under: "
                 f"'{os.path.abspath(os.path.dirname(path))}'\n")