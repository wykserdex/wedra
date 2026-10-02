#!/usr/bin/env python3
"""Mock CLI ScoutSuite (`scout`) для контракт-тестов (без сети и без пакета).

Имитирует `scout <provider> --report-dir <d> --report-name <n>
--result-format json --no-browser [--profile P]`: раскладывает отчёт как
ScoutSuite — <report-dir>/scoutsuite-results/scoutsuite_results_<name>.js, где
первая строка `scoutsuite_results =`, а дальше JSON-объект с
services[<service>].findings[<id>] = {level, description, items}. Режимы env:
MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (тест no_report),
MOCK_FAIL=1 (ненулевой код → тест tool_failed), MOCK_BAD_REPORT=1 (битый JSON →
тест bad_report), MOCK_EMPTY=1 (отчёт без находок),
MOCK_STRICT_ARGS=1 (флаги --report-dir/--report-name/--result-format/--no-browser
обязательны, а --profile принимается только при provider=aws).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("ERROR scout.py L104: Authentication failure: no credentials",
          file=sys.stderr)
    sys.exit(101)

argv = sys.argv[1:]
if argv and not argv[0].startswith("-"):
    provider = argv[0]
    flags = argv[1:]
else:
    provider = ""
    flags = argv

values = {}
i = 0
while i < len(flags):
    if flags[i].startswith("--") and i + 1 < len(flags):
        values[flags[i]] = flags[i + 1]
        i += 2
        continue
    i += 1

if not provider:
    print("usage: scout [-h] ...", file=sys.stderr)
    print("error: the following arguments are required: provider", file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_STRICT_ARGS") == "1":
    missing = [f for f in ("--report-dir", "--report-name", "--result-format")
               if f not in values]
    if "--no-browser" not in flags:
        missing.append("--no-browser")
    if missing:
        print("scout: missing flags %s" % ",".join(missing), file=sys.stderr)
        sys.exit(7)
    if values.get("--result-format") != "json":
        print("scout: --result-format must be json", file=sys.stderr)
        sys.exit(8)
    if "--profile" in values and provider != "aws":
        print("scout: --profile is not available for %s" % provider,
              file=sys.stderr)
        sys.exit(9)

print("INFO scout.py L1: Launching Scout", file=sys.stderr)
print("INFO scout.py L2: Authenticating to cloud provider", file=sys.stderr)
print("INFO scout.py L3: Gathering data from APIs", file=sys.stderr)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("INFO scout.py L10: report not written", file=sys.stderr)
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    report = {"service_name": "ScoutSuite", "provider": provider,
              "services": {"ec2": {"findings": {}}}}
else:
    report = {
        "service_name": "ScoutSuite",
        "provider": provider,
        "cloud_config": {"account_id": "123456789012"},
        "services": {
            "ec2": {
                "findings": {
                    "instancesWithPublicIp": {
                        "level": "danger",
                        "description": "EC2 instances with public IP",
                        "items": [{"name": "i-0demo"}, {"name": "i-0other"}],
                    },
                    "defaultSecurityGroup": {
                        "level": "warning",
                        "description": "Default security group in use",
                        "items": [{"name": "default"}],
                    },
                    "unusedSecurityGroups": {
                        "level": "info",
                        "description": "Unused security groups",
                        "items": [],
                    },
                },
            },
            "iam": {
                "findings": {
                    "rootAccount": {
                        "level": "danger",
                        "description": "Root account active",
                        "items": [{"name": "<root_account>"}],
                    },
                },
            },
        },
    }

report_dir = values.get("--report-dir", "scoutsuite-report")
report_name = values.get("--report-name", "report")
results_dir = os.path.join(report_dir, "scoutsuite-results")
os.makedirs(results_dir, exist_ok=True)
path = os.path.join(results_dir, "scoutsuite_results_%s.js" % report_name)

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(path, "w", encoding="utf-8") as f:
        f.write("scoutsuite_results =\n{\"services\": {\"ec2\": {\"findings\": ")
    sys.exit(0)

with open(path, "w", encoding="utf-8") as f:
    f.write("scoutsuite_results =\n")
    json.dump(report, f)

html_path = os.path.join(report_dir, "%s-%s.html" % (provider, report_name))
with open(html_path, "w", encoding="utf-8") as f:
    f.write("<html><body>scout report</body></html>\n")

print("INFO scout.py L11: Report saved to %s" % html_path, file=sys.stderr)