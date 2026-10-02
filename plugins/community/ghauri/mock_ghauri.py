#!/usr/bin/env python3
"""Mock CLI ghauri для контракт-тестов (без сети и без пакета ghauri).

Имитирует ghauri: читает цель из -u, пишет текстовый лог .ghauri/<netloc>/log
относительно CWD (main.py подменяет HOME/USERPROFILE на временный каталог, а
CWD и есть этот каталог). В лог кладём ANSI-последовательности, как их оставляет
colorize() в реальном ghauri.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_FAIL=1 (ненулевой код),
MOCK_NO_REPORT=1 (нет лога), MOCK_NO_PARAMS=1 (в URL нет параметров),
MOCK_NOT_VULNERABLE=1 (параметры не инъекционны).
"""
import os
import sys
import time
from urllib.parse import urlparse

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    sys.exit(2)

args = sys.argv[1:]
url = args[args.index("-u") + 1] if "-u" in args else "http://example.com/"

netloc = urlparse(url).netloc.split(":")[0]
lines = [
    "\n[*] starting @ 12:00:00 /2026-01-01/",
    f"using back-end 'MySQL' ...",
    "GET parameter 'id' (1) does not seem to be injectable",
]

if os.environ.get("MOCK_NO_PARAMS") == "1":
    lines = [
        "\n[*] starting @ 12:00:00 /2026-01-01/",
        "no parameter(s) found for testing in the provided data (e.g. GET "
        "parameter 'id' in 'www.site.com/index.php?id=1')",
        "\n[*] ending @ 12:00:01 /2026-01-01/",
    ]
elif os.environ.get("MOCK_NOT_VULNERABLE") == "1":
    lines.append(
        "heuristic (basic) test shows that GET parameter "
        "'\x1b[0m\x1b[2m\x1b[37mid\x1b[0m\x1b[22m\x1b[39m' might not be "
        "injectable")
    lines.append("all tested parameters do not appear to be injectable.")
else:
    lines = [
        "\n[*] starting @ 12:00:00 /2026-01-01/",
        "heuristic (basic) test shows that GET parameter "
        "'\x1b[0m\x1b[2m\x1b[37mid\x1b[0m\x1b[22m\x1b[39m' might be "
        "injectable (possible DBMS: 'MySQL')",
        "GET parameter '\x1b[0m\x1b[2m\x1b[37mid\x1b[0m\x1b[22m\x1b[39m' "
        "appears to be '\x1b[0m\x1b[2m\x1b[37mOR boolean-based blind - "
        "WHERE or HAVING clause\x1b[0m\x1b[22m\x1b[39m' injectable",
        "POST parameter '\x1b[0m\x1b[2m\x1b[37mname\x1b[0m\x1b[22m\x1b[39m' "
        "appears to be '\x1b[0m\x1b[2m\x1b[37mMySQL inline query "
        "(QUOTE)\x1b[0m\x1b[22m\x1b[39m' injectable",
        "\n[*] ending @ 12:00:07 /2026-01-01/",
    ]

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] log not written", file=sys.stderr)
    sys.exit(0)

path = os.path.join(".ghauri", netloc, "log")
os.makedirs(os.path.dirname(path), exist_ok=True)
with open(path, "w", encoding="utf-8") as f:
    f.write("\n".join(lines) + "\n")
print(f"[*] fetched data logged to text files under: "
      f"'{os.path.abspath(os.path.dirname(path))}'", file=sys.stderr)