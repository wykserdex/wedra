#!/usr/bin/env python3
"""Mock публичного API instaloader (паттерн C) для контракт-тестов.

Имитирует сниппет плагина: берёт имя профиля из argv и печачает в stdout один
JSON-объект метаданных профиля, как это делает instaloader
(Profile.from_username). Ни сети, ни пакета instaloader, ни логина. Режимы env:
MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout, тест
no_report), MOCK_FAIL=1 (ProfileNotExistsException, тест tool_failed),
MOCK_FAIL_RETRY=1 (HTTP Error 429, tool_failed c retryable), MOCK_BAD_REPORT=1
(битый JSON), MOCK_PRIVATE=1 (закрытый профиль), MOCK_MISSING_FIELDS=1 (профиль
без полей — все счётчики 0 и пустые строки).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

args = sys.argv[1:]
profile = next((a for a in args if not a.startswith("-")), "")

if not profile:
    print("usage: instaloader profile", file=sys.stderr)
    sys.exit(2)

if os.environ.get("MOCK_FAIL_RETRY") == "1":
    print("HTTP Error 429: Too Many Requests", file=sys.stderr)
    sys.exit(1)
if os.environ.get("MOCK_FAIL") == "1":
    print(f"ProfileNotExistsException: Profile {profile} does not exist.",
          file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

if os.environ.get("MOCK_PRIVATE") == "1":
    report = {"username": profile, "full_name": "", "biography": "",
              "followers": 0, "followees": 0, "posts": 0, "is_private": True,
              "profile_pic_url": ""}
elif os.environ.get("MOCK_MISSING_FIELDS") == "1":
    report = {"username": profile}
else:
    report = {
        "username": profile,
        "full_name": "Demo Person",
        "biography": "демо-профиль",
        "followers": 12345,
        "followees": 321,
        "posts": 678,
        "is_private": False,
        "profile_pic_url": "https://example.com/avatar.jpg",
    }

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{"username": "demo", "followers": 12345')
    sys.exit(0)

sys.stdout.write(json.dumps(report, ensure_ascii=False) + "\n")
