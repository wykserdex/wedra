#!/usr/bin/env python3
"""Mock exiftool CLI для контракт-тестов (без сети и без пакета exiftool).

Имитирует `exiftool -j -q <file>`: JSON-массив с тегами печатается в stdout,
счётчик прочитанных файлов — в stderr. Все значения тегов выдуманные: EXIF
может содержать персональные данные (GPS, устройство), в тестах только моки.
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест
no_report), MOCK_FAIL=1 (ненулевой код), MOCK_BAD_REPORT=1 (битый JSON).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("Error: Unknown option -qq", file=sys.stderr)
    sys.exit(2)

args = sys.argv[1:]
target = args[-1] if args else "mock.jpg"

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('[{"SourceFile": "mock.jpg", "Make": }]', flush=True)
    sys.exit(0)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("1 image files read", file=sys.stderr)
    sys.exit(0)

report = [
    {
        "SourceFile": target,
        "FileType": "JPEG",
        "FileSize": 20480,
        "Make": "MOCKCAMERA",
        "Model": "MockModel X1",
        "DateTimeOriginal": "2024:01:02 03:04:05",
        "GPSLatitude": "55.7558",
        "GPSLongitude": "37.6173",
    },
]
print(json.dumps(report, ensure_ascii=False))
print("1 image files read", file=sys.stderr)