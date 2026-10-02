#!/usr/bin/env python3
"""Mock CLI yt-dlp для контракт-тестов (без сети и без пакета yt-dlp).

Имитирует -J/--dump-single-json: печатает в stdout один JSON-объект info-репорта
с жирными полями formats/thumbnails/description — плагин обязан вырезать из
него только нужные поля. Как и настоящий yt-dlp, требует в argv -J и
--skip-download, иначе ERROR в stderr и ненулевой exit. Если в argv есть
--yes-playlist, печатает playlist-репорт (у него нет полей duration/uploader/
formats). Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой
stdout, тест no_report), MOCK_FAIL=1 (тест tool_failed), MOCK_FAIL_RETRY=1
(сетевая ошибка 429, tool_failed c retryable), MOCK_BAD_REPORT=1 (битый JSON),
MOCK_MINIMAL=1 (в отчёте только webpage_url).
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
    print("ERROR: unable to download video data: HTTP Error 429: "
          "Too Many Requests", file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_FAIL") == "1":
    print(f"ERROR: Unsupported URL: {url}", file=sys.stderr)
    sys.exit(1)

if "-J" not in args and "--dump-single-json" not in args:
    print("ERROR: --dump-single-json is required for metadata mode",
          file=sys.stderr)
    sys.exit(2)
if "--skip-download" not in args:
    print("ERROR: refusing to run without --skip-download", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_MINIMAL") == "1":
    report = {"_type": "video", "id": "demo", "webpage_url": url}
elif "--yes-playlist" in args:
    report = {
        "_type": "playlist",
        "id": "PLdemo",
        "title": "Demo Playlist",
        "extractor": "youtube",
        "extractor_key": "Youtube",
        "webpage_url": url,
        "playlist_count": 2,
        "entries": [
            {"_type": "video", "id": "one", "title": "Demo Video One"},
            {"_type": "video", "id": "two", "title": "Demo Video Two"},
        ],
    }
else:
    report = {
        "_type": "video",
        "id": "dQw4w9WgXcQ",
        "title": "Demo Video",
        "uploader": "Demo Channel",
        "channel": "Demo Channel",
        "uploader_id": "@demochannel",
        "duration": 212,
        "extractor": "youtube",
        "extractor_key": "Youtube",
        "webpage_url": url,
        "description": "демонстрационное описание ролика",
        "thumbnail": "https://example.com/thumb.jpg",
        "formats": [
            {"format_id": "18", "ext": "mp4", "width": 640, "height": 360},
            {"format_id": "137", "ext": "mp4", "width": 1920, "height": 1080},
            {"format_id": "140", "ext": "m4a", "abr": 128},
        ],
        "requested_formats": [
            {"format_id": "137"}, {"format_id": "140"},
        ],
        "thumbnails": [{"url": f"https://example.com/t{i}.jpg",
                        "width": 120 * i} for i in range(1, 11)],
        "automatic_captions": {"en": [{"url": "https://example.com/c.vtt"}]},
    }

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{"title": "Demo Video", "formats": [{"format_id": 18}')
    sys.exit(0)

sys.stdout.write(json.dumps(report, ensure_ascii=False) + "\n")
print("[debug] Downloading json", file=sys.stderr)
