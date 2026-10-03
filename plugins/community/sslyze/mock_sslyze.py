#!/usr/bin/env python3
"""Mock sslyze CLI для контракт-тестов (без сети и без пакета sslyze).

Имитирует sslyze: читает --json_out <файл> --quiet [--<команда> ...] <target>
и пишет в файл из --json_out документ вида
{"server_scan_results": [{server_location{hostname,port,ip_address},
scan_status, connectivity_error_trace, scan_result{<команда>: {status,
result}}}]}. Ни одного TLS-соединения не устанавливается. scan_status —
допустимое значение ServerScanStatusEnum реального sslyze (COMPLETED или
ERROR_NO_CONNECTIVITY).

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (нет файла
--json_out), MOCK_FAIL=1 (ненулевой код выхода), MOCK_BAD_REPORT=1 (битый
JSON), MOCK_EMPTY=1 (server_scan_results: []), MOCK_NO_CONNECT=1 (сервер без
связности: scan_status ERROR_NO_CONNECTIVITY).
"""
import json
import os
import sys
import time


def opt(argv, flag):
    for i, arg in enumerate(argv):
        if arg == flag and i + 1 < len(argv):
            return argv[i + 1]
        if arg.startswith(flag + "="):
            return arg.split("=", 1)[1]
    return None


if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

argv = sys.argv[1:]
report_path = opt(argv, "--json_out")
target = None
for arg in argv:
    if not arg.startswith("-") and arg != report_path:
        target = arg
        break
if not report_path or not target:
    print("sslyze: the following arguments are required: target",
          file=sys.stderr)
    sys.exit(2)

commands = [arg.lstrip("-") for arg in argv if arg.startswith("--")
            and arg.split("=")[0] not in ("--json_out", "--quiet")]

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)
if os.environ.get("MOCK_FAIL") == "1":
    print("sslyze: Could not resolve hostname: " + target, file=sys.stderr)
    sys.exit(2)

hostname, _, port = target.rpartition(":")
hostname = hostname or target
port = int(port) if port.isdigit() else 443

if os.environ.get("MOCK_EMPTY") == "1":
    report = {"server_scan_results": []}
else:
    scan_result = {}
    for command in commands:
        scan_result[command.replace("-", "_")] = {
            "status": "OK",
            "result": {"scanned_commands": [command]},
        }
    if os.environ.get("MOCK_NO_CONNECT") == "1":
        scan_result = {}
    server = {
        "server_location": {"hostname": hostname, "port": port,
                            "ip_address": "192.0.2.10",
                            "reverse_hostname": None},
        "scan_status": ("ERROR_NO_CONNECTIVITY"
                        if os.environ.get("MOCK_NO_CONNECT") == "1"
                        else "COMPLETED"),
        "connectivity_error_trace": ("socket.gaierror: Name or service not known"
                                     if os.environ.get("MOCK_NO_CONNECT") == "1"
                                     else None),
        "scan_result": scan_result,
        "network_configuration": {},
    }
    report = {"server_scan_results": [server], "version": "6.0.0"}

with open(report_path, "w", encoding="utf-8") as f:
    if os.environ.get("MOCK_BAD_REPORT") == "1":
        f.write("{не json\n")
    else:
        f.write(json.dumps(report, ensure_ascii=False, indent=2) + "\n")

print("RESULTS: %d server(s) scanned" % len(report["server_scan_results"]),
      file=sys.stderr)