#!/usr/bin/env python3
"""Mock droopescan CLI для контракт-тестов (без сети и без пакета droopescan).

Имитирует `droopescan scan [cms] -u <url> --output json`: печатает в stdout один
JSON-объект репорта {host, cms_name, version, plugins, themes, interesting
urls} — ровно как JsonOutput.result — и только если что-то найдено; при
MOCK_NO_REPORT печатает в stdout сообщение «not identified as a supported CMS»
и выходит с кодом 0 (так ведёт себя настоящий при RuntimeError).
Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1,
MOCK_BAD_REPORT=1 (битая JSON-строка), MOCK_FAIL=1 (ненулевой код выхода).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("[+] Connection error: connection refused", file=sys.stderr)
    sys.exit(1)

argv = sys.argv[1:]
if not argv or argv[0] != "scan":
    print("usage: droopescan scan [-u URL] [--output standard|json]",
          file=sys.stderr)
    sys.exit(1)


def flag(*names):
    for i, arg in enumerate(argv):
        for name in names:
            if arg == name and i + 1 < len(argv):
                return argv[i + 1]
            if arg.startswith(name + "="):
                return arg.split("=", 1)[1]
    return None


target = flag("-u", "--url")
if not target:
    print("+ --url parameter is required.", file=sys.stderr)
    sys.exit(1)

VALUE_FLAGS = {"-u", "--url", "-U", "--url-file", "-e", "--enumerate",
               "-n", "--number", "-t", "--threads", "--output", "-o",
               "--method", "--verb", "--timeout", "--host", "--user-agent",
               "--error-log"}
cms = "drupal"
i = 1
while i < len(argv):
    arg = argv[i]
    if arg.startswith("-"):
        i += 2 if arg in VALUE_FLAGS else 1
        continue
    cms = arg
    break

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("'%s' not identified as a supported CMS. If you disagree, please "
          "specify a CMS manually." % target)
    sys.exit(0)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    print('{"host": "%s", "version": {' % target)
    sys.exit(0)

base = target.rstrip("/")
report = {
    "host": target if target.endswith("/") else target + "/",
    "cms_name": cms,
    "version": {"is_empty": False, "finds": ["7.34", "7.35", "7.36"]},
    "plugins": {"is_empty": False, "finds": [
        {"url": base + "/sites/all/modules/views/", "name": "views"},
        {"url": base + "/sites/all/modules/pathauto/", "name": "pathauto"},
        {"url": base + "/sites/all/modules/token/", "name": "token"},
    ]},
    "themes": {"is_empty": False, "finds": [
        {"url": base + "/themes/garland/", "name": "garland"},
    ]},
    "interesting urls": {"is_empty": False, "finds": [
        {"url": base + "/CHANGELOG.txt", "description": "Default changelog file."},
    ]},
}
print(json.dumps(report))
print("[+] Scan finished", file=sys.stderr)