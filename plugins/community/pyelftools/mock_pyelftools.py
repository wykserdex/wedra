#!/usr/bin/env python3
"""Mock донора pyelftools (паттерн C) для контракт-тестов — без пакета elftools.

main.py запускает `<PYELFTOOLS_BIN> <путь к файлу>`; мок печатает в stdout тот
же JSON, что печатает SNIPPET из main.py: class, machine, type, entry,
sections[], symbols[] (символы в выдаче есть, наружу плагин их не отдаёт) и
imported_libraries[]. Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_NOT_ELF=1
(доменная ошибка not_elf), MOCK_BAD_FILE=1 (bad_file), MOCK_NO_MODULE=1 (донор
не установлен), MOCK_NO_REPORT=1 (пустой stdout), MOCK_FAIL=1 (ненулевой код),
MOCK_BAD_REPORT=1 (битый JSON), MOCK_ALT=1 (ELF32, секций нет, библиотек нет).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

target = sys.argv[1] if len(sys.argv) > 1 else ""
print("pyelftools mock: разбираю %s" % target, file=sys.stderr)

if os.environ.get("MOCK_NOT_ELF") == "1":
    print(json.dumps({"wedra_error": "not_elf",
                      "wedra_message": "файл не читается как ELF: Invalid ELF magic"}))
    sys.exit(1)
if os.environ.get("MOCK_BAD_FILE") == "1":
    print(json.dumps({"wedra_error": "bad_file",
                      "wedra_message": "не удалось разобрать ELF: truncated section header"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_MODULE") == "1":
    print(json.dumps({"wedra_error": "pyelftools_not_installed",
                      "wedra_message": "пакет pyelftools не импортируется: No module named 'elftools'"}))
    sys.exit(1)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    sys.stderr.write("pyelftools mock: упал с ошибкой\n")
    sys.exit(2)
if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write("{\"class\": \"ELF64\", \"sections\": [,,]}\n")
    sys.exit(0)

if os.environ.get("MOCK_ALT") == "1":
    print(json.dumps({"class": "ELF32", "machine": "EM_386", "type": "ET_REL",
                      "entry": 0, "sections": [], "symbols": [],
                      "imported_libraries": []}))
    sys.exit(0)

print(json.dumps({
    "class": "ELF64",
    "machine": "EM_X86_64",
    "type": "ET_DYN",
    "entry": 10506752,
    "sections": [
        {"name": ".interp", "type": "SHT_PROGBITS", "size": 28, "addr": 568},
        {"name": ".text", "type": "SHT_PROGBITS", "size": 48213, "addr": 10768},
        {"name": ".dynamic", "type": "SHT_DYNAMIC", "size": 472, "addr": 488544},
    ],
    "symbols": [
        {"name": "printf", "type": "STT_FUNC", "value": 24192, "size": 128},
        {"name": "greet", "type": "STT_FUNC", "value": 24480, "size": 64},
    ],
    "imported_libraries": ["libc.so.6", "libm.so.6"],
}))