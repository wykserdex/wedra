#!/usr/bin/env python3
"""Mock донора lief (паттерн C) для контракт-тестов — без пакета lief.

main.py запускает `<LIEF_BIN> <путь к файлу>`; мок печатает в stdout тот же
JSON, что печатает SNIPPET из main.py: format, header{}, sections[],
section_count. Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_UNSUPPORTED=1
(доменная ошибка unsupported_format), MOCK_BAD_FILE=1 (формат опознан, но файл
не разобран), MOCK_NO_MODULE=1 (донор не установлен), MOCK_NO_REPORT=1 (пустой
stdout), MOCK_FAIL=1 (ненулевой код), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_ALT=1 (ELF вместо PE, секций нет), MOCK_LEGACY=1 (старый LIEF без
абстрактного header — поля пустые).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

target = sys.argv[1] if len(sys.argv) > 1 else ""
print("lief mock: разбираю %s" % target, file=sys.stderr)

if os.environ.get("MOCK_UNSUPPORTED") == "1":
    print(json.dumps({"wedra_error": "unsupported_format",
                      "wedra_message": "LIEF не поддерживает формат файла: notes.txt"}))
    sys.exit(1)
if os.environ.get("MOCK_BAD_FILE") == "1":
    print(json.dumps({"wedra_error": "bad_file",
                      "wedra_message": "LIEF не смог разобрать файл: corrupted section table"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_MODULE") == "1":
    print(json.dumps({"wedra_error": "lief_not_installed",
                      "wedra_message": "пакет lief не импортируется: No module named 'lief'"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("lief mock: упал с ошибкой\n")
    sys.exit(2)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write("{\"format\": \"PE\", \"sections\": [,,]}\n")
    sys.exit(0)

if os.environ.get("MOCK_LEGACY") == "1":
    print(json.dumps({"format": "PE",
                      "header": {"architecture": "", "object_type": "",
                                 "endianness": "", "is_64": False,
                                 "entrypoint": 0},
                      "sections": [], "section_count": 0}))
    sys.exit(0)

if os.environ.get("MOCK_ALT") == "1":
    print(json.dumps({"format": "ELF",
                      "header": {"architecture": "X86_64",
                                 "object_type": "LIBRARY",
                                 "endianness": "LITTLE", "is_64": True,
                                 "entrypoint": 10506752},
                      "sections": [], "section_count": 0}))
    sys.exit(0)

print(json.dumps({
    "format": "PE",
    "header": {"architecture": "X86_64", "object_type": "EXECUTABLE",
               "endianness": "LITTLE", "is_64": True,
               "entrypoint": 4198400},
    "sections": [
        {"name": ".text", "size": 4198400, "offset": 1024,
         "virtual_address": 4198400},
        {"name": ".rdata", "size": 65536, "offset": 4206592,
         "virtual_address": 4259840},
    ],
    "section_count": 2,
}))