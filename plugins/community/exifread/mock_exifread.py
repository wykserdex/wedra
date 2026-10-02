#!/usr/bin/env python3
"""Mock exifread-сниппет для контракт-тестов (без пакета exifread, без сети).

Стоит на место `python -c <SNIPPET> <файл>`: печатает в stdout тот же JSON
{file, tags, count}, который печатает боевой сниппет. Состав тегов зависит от MOCK_NO_TAGS=1 или имени файла. Режимы env: MOCK_SLEEP=N
(тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout), MOCK_FAIL=1 (ненулевой
код выхода), MOCK_BAD_REPORT=1 (битый JSON), MOCK_NO_MODULE=1 (имитация
«No module named exifread»).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

path = sys.argv[1] if len(sys.argv) > 1 else ""

if os.environ.get("MOCK_FAIL") == "1":
    print("mock: не смог прочитать файл", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_MODULE") == "1":
    print("ModuleNotFoundError: No module named 'exifread'", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("[*] exifread: нечего печатать", file=sys.stderr)
    sys.exit(0)

if not os.path.isfile(path):
    print("mock: файла нет: %s" % path, file=sys.stderr)
    sys.exit(1)

tags = {}
if os.environ.get("MOCK_NO_TAGS") != "1" and "no-exif" not in os.path.basename(path).lower():
    tags = {
        "Image Make": "Canon",
        "Image Model": "Canon EOS 5D Mark IV",
        "EXIF DateTimeOriginal": "2024:05:17 11:38:32",
        "GPS GPSLatitudeRef": "N",
    }

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"file": "%s", "tags": {"Image Make": tru' % path)
    sys.exit(0)

print(json.dumps({"file": path, "tags": tags, "count": len(tags)}))
print("[*] exifread mock: %d тегов из %s" % (len(tags), path),
      file=sys.stderr)
