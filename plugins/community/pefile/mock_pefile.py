#!/usr/bin/env python3
"""Mock донора pefile (паттерн C) для контракт-тестов — без пакета pefile.

main.py запускает `<PEFILE_BIN> <путь к файлу>`; этот мок печатает в stdout тот
же компактный JSON, что печатает SNIPPET из main.py: machine (число),
sections[] и imports[]. Режимы env: MOCK_SLEEP=N (wall_timeout),
MOCK_NOT_PE=1 (доменная ошибка not_pe), MOCK_BAD_FILE=1 (bad_file),
MOCK_NO_PE_MODULE=1 (donor не установлен), MOCK_NO_REPORT=1 (пустой stdout),
MOCK_FAIL=1 (ненулевой код), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_ALT=1 (другая архитектура, пустые импорты).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

target = sys.argv[1] if len(sys.argv) > 1 else ""
print("pefile mock: разбираю %s" % target, file=sys.stderr)

if os.environ.get("MOCK_NOT_PE") == "1":
    print(json.dumps({"wedra_error": "not_pe",
                      "wedra_message": "файл не читается как PE: Invalid DOS Signature"}))
    sys.exit(1)
if os.environ.get("MOCK_BAD_FILE") == "1":
    print(json.dumps({"wedra_error": "bad_file",
                      "wedra_message": "не удалось разобрать PE: truncated header"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_PE_MODULE") == "1":
    print(json.dumps({"wedra_error": "pefile_not_installed",
                      "wedra_message": "пакет pefile не импортируется: No module named 'pefile'"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("pefile mock: упал с ошибкой\n")
    sys.exit(2)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write("{\"machine\": 34404, \"sections\": [,,]}\n")
    sys.exit(0)

if os.environ.get("MOCK_ALT") == "1":
    print(json.dumps({"machine": 358, "sections": [], "imports": []}))
    sys.exit(0)

print(json.dumps({
    "machine": 34404,
    "sections": [
        {"name": ".text", "virtual_address": 4096, "size": 1048576,
         "characteristics": 2164261632},
        {"name": ".rdata", "virtual_address": 1052672, "size": 65536,
         "characteristics": 1073741824},
        {"name": ".rsrc", "virtual_address": 1118208, "size": 2048,
         "characteristics": 1073741824},
    ],
    "imports": [
        {"dll": "KERNEL32.dll", "symbols": ["CreateFileW", "GetProcAddress"]},
        {"dll": "USER32.dll", "symbols": ["MessageBoxW"]},
    ],
}))