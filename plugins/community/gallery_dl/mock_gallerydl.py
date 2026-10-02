#!/usr/bin/env python3
"""Mock CLI gallery-dl для контракт-тестов (без сети и без пакета gallery-dl).

Имитирует -j/--dump-json: печатает в stdout JSON-массив сообщений DataJob
(gallery_dl/job.py + gallery_dl/extractor/message.py) — [2, <метаданные
галереи>] и [3, <url картинки>, <метаданные>]. Как и настоящий gallery-dl в
режиме метаданных, требует в argv -j и --no-download, иначе ERROR в stderr и
ненулевой exit. Режимы env: MOCK_SLEEP=N (wall_timeout), MOCK_NO_REPORT=1
(пустой stdout, тест no_report), MOCK_FAIL=1 (тест tool_failed),
MOCK_FAIL_RETRY=1 (ошибка 429, tool_failed c retryable), MOCK_BAD_REPORT=1
(битый JSON), MOCK_EMPTY=1 (галерея без картинок), MOCK_NO_DIRECTORY=1 (нет
сообщения Message.Directory), MOCK_JSONL=1 (вывод output.jsonl — по объекту
на строку).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
url = next((a for a in args if not a.startswith("-")), "")

if os.environ.get("MOCK_FAIL_RETRY") == "1":
    print(f"{url or 'https://example.com'}: HTTP Error 429: Too Many Requests",
          file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_FAIL") == "1":
    print(f"{url or 'https://example.com'}: unsupported URL", file=sys.stderr)
    sys.exit(1)

if "-j" not in args and "--dump-json" not in args:
    print("usage: gallery-dl [-j] URL", file=sys.stderr)
    sys.exit(2)
if "--no-download" not in args:
    print("ERROR: refusing to run without --no-download", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

directory = {"category": "Demo Gallery", "title": "Demo Gallery",
             "date": "2024-01-02", "count": 3}
items = [
    [3, "https://example.com/img1.jpg",
     {"category": "Demo Gallery", "extension": "jpg", "num": 1,
      "width": 1200, "height": 1600, "artist": "Demo Artist"}],
    [3, "https://example.com/img2.jpg",
     {"category": "Demo Gallery", "extension": "jpg", "num": 2,
      "width": 1200, "height": 1600}],
    [3, "https://example.com/img3.png",
     {"category": "Demo Gallery", "extension": "png", "num": 3,
      "width": 800, "height": 600}],
]

report = []
if os.environ.get("MOCK_NO_DIRECTORY") != "1":
    report.append([2, directory])
if os.environ.get("MOCK_EMPTY") != "1":
    report += items

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('[[3, "https://example.com/img1.jpg", {"num": 1}')
    sys.exit(0)

if os.environ.get("MOCK_JSONL") == "1":
    for message in report:
        sys.stdout.write(json.dumps(message, ensure_ascii=False) + "\n")
else:
    json.dump(report, sys.stdout, ensure_ascii=False, indent=2, sort_keys=True)
    sys.stdout.write("\n")
print("[debug] dumping JSON output", file=sys.stderr)
