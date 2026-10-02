#!/usr/bin/env python3
"""Mock binwalk CLI v3 для контракт-тестов (без пакета binwalk).

Имитирует v3: читает `--log <файл> --quiet [--extract --directory <каталог>]
<файл>`, пишет по пути из --log JSON-массив объектов {"Analysis": {file_path,
file_map, extractions}} и при --extract создаёт файлы в каталоге из --extract
(поддиректория по HEX-смещению, как у настоящего). Файл на входе должен
существовать. Пустой отчёт — по MOCK_NO_SIGS=1. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (нет файла отчёта), MOCK_FAIL=1 (ненулевой код
выхода), MOCK_BAD_REPORT=1 (битый JSON), MOCK_EXTRACT_FAIL=1 (экстракция без
успеха), MOCK_EXTRACT_EMPTY=1 (экстракция ничего не дала).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
log_path = ""
extract_dir = ""
do_extract = False
target = ""
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("--log", "-l") and i + 1 < len(args):
        log_path = args[i + 1]
        i += 2
        continue
    if arg in ("--directory", "-d") and i + 1 < len(args):
        extract_dir = args[i + 1]
        i += 2
        continue
    if arg in ("--extract", "-e"):
        do_extract = True
        i += 1
        continue
    if arg.startswith("-"):
        i += 1
        continue
    target = arg
    i += 1

print("DECIMAL       HEXADECIMAL     DESCRIPTION")

if os.environ.get("MOCK_FAIL") == "1":
    print("mock: искусственная ошибка", file=sys.stderr)
    sys.exit(1)

if not target or not os.path.isfile(target):
    print("mock: файла нет: %s" % target, file=sys.stderr)
    sys.exit(1)

if not log_path:
    print("mock: не задан --log", file=sys.stderr)
    sys.exit(1)

file_map = []
extractions = {}

if os.environ.get("MOCK_NO_SIGS") != "1":
    file_map = [
        {"offset": 0,
         "id": "11111111-1111-1111-1111-111111111111",
         "size": 4096, "name": "gzip", "confidence": 250,
         "description": "gzip compressed data, from Unix, original size modulo 2^32 8192",
         "always_display": True, "extraction_declined": False},
        {"offset": 8192,
         "id": "22222222-2222-2222-2222-222222222222",
         "size": 2048, "name": "lzma", "confidence": 128,
         "description": "LZMA compressed data, size: 2048",
         "always_display": False, "extraction_declined": False},
    ]

    if do_extract:
        first_ok = (os.environ.get("MOCK_EXTRACT_FAIL") != "1"
                    and os.environ.get("MOCK_EXTRACT_EMPTY") != "1")
        outdir = os.path.join(extract_dir, os.path.basename(target),
                              "0") if extract_dir else os.path.join(
            os.getcwd(), os.path.basename(target), "0")
        os.makedirs(outdir, exist_ok=True)
        if first_ok:
            with open(os.path.join(outdir, "decompressed.bin"), "wb") as f:
                f.write(b"\x1f\x8b\x08\x00mock-extracted-payload")
        extractions[file_map[0]["id"]] = {
            "size": 4096, "success": first_ok,
            "extractor": "gzip_built_in", "do_not_recurse": False,
            "output_directory": os.path.abspath(outdir),
        }
        extractions[file_map[1]["id"]] = {
            "size": 2048, "success": False,
            "extractor": "lzma_built_in", "do_not_recurse": False,
            "output_directory": os.path.abspath(outdir),
        }

report = [{"Analysis": {"file_path": os.path.abspath(target),
                        "file_map": file_map,
                        "extractions": extractions}}]

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(log_path, "w", encoding="utf-8") as f:
        f.write('[{"Analysis": {"file_map": [{"offset": 0, tru')
    print("[*] JSON-отчёт записан: %s (битый)" % log_path, file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] отчёт не задан: симулируем обрыв", file=sys.stderr)
    sys.exit(0)

for sig in file_map:
    print("%-12d 0x%-12X %s" % (sig["offset"], sig["offset"],
                                sig["description"]))
print("[*] JSON-отчёт записан: %s" % log_path, file=sys.stderr)
with open(log_path, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
