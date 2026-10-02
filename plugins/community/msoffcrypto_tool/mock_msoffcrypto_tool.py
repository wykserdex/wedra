#!/usr/bin/env python3
"""Mock CLI msoffcrypto-tool для контракт-тестов (без пакета, без документов).

Имитирует `msoffcrypto-tool <infile> <outfile> -p <password>` и режим проверки
`-t -v <infile>`: пишет в outfile заглушку «расшифрованного» документа,
человекочитаемый текст — в stderr. Пароль из argv наружу НЕ печатается.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (не создаёт
outfile), MOCK_FAIL=1 (ненулевой код выхода), MOCK_NOT_ENCRYPTED=1 (режим -t
находит незашифрованный документ → exit 1).
"""
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
test_mode = "-t" in args or "--test" in args
positional = [a for a in args if not a.startswith("-")]

if os.environ.get("MOCK_FAIL") == "1":
    print("msoffcrypto-tool: InvalidKeyError: failed to decrypt",
          file=sys.stderr)
    sys.exit(1)

if test_mode:
    infile = positional[0] if positional else "document.doc"
    if os.environ.get("MOCK_NOT_ENCRYPTED") == "1":
        print("%s: not encrypted" % infile, file=sys.stderr)
        sys.exit(1)
    print("%s: encrypted" % infile, file=sys.stderr)
    print("msoffcrypto-tool 5.4.2 (mock)", file=sys.stderr)
    sys.exit(0)

if len(positional) < 2:
    print("msoffcrypto-tool: нужны infile и outfile", file=sys.stderr)
    sys.exit(2)

infile, outfile = positional[0], positional[1]
print("[+] Decrypting %s (password taken from -p)" % infile, file=sys.stderr)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

with open(outfile, "wb") as f:
    f.write(b"MOCK-DECRYPTED-DOCUMENT\n")
print("[+] Decrypted document written to %s" % outfile, file=sys.stderr)