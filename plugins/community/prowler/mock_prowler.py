#!/usr/bin/env python3
"""Mock CLI prowler для контракт-тестов (без сети и без пакета).

Имитирует `prowler <provider> -M json-ocsf -o <dir> -F <name> -b -z
[--severity S]`: раскладывает отчёт так же, как prowler/config/config.py —
<dir>/<name>.ocsf.json, содержимое — JSON-массив OCSF Detection Finding
(metadata.event_code = CheckID, finding_info.title = CheckTitle,
severity, status_code). Режимы env: MOCK_SLEEP=N (тест wall_timeout),
MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (ненулевой код → тест
tool_failed), MOCK_BAD_REPORT=1 (битый JSON → тест bad_report), MOCK_EMPTY=1
(пустой массив), MOCK_STRICT_ARGS=1 (флаги -M json-ocsf / -o / -F / -b / -z
обязательны, --severity допустим только из списка).
"""
import json
import os
import sys
import time

SEVERITIES = ("critical", "high", "medium", "low", "informational")

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("ERROR: provider aws is not supported by this installation",
          file=sys.stderr)
    sys.exit(1)

argv = sys.argv[1:]
if not argv or argv[0].startswith("-"):
    print("usage: prowler [-h] provider", file=sys.stderr)
    print("prowler: error: the following arguments are required: provider",
          file=sys.stderr)
    sys.exit(2)
provider = argv[0]

values = {}
switches = []
i = 1
while i < len(argv):
    arg = argv[i]
    if arg.startswith("-") and i + 1 < len(argv) and not argv[i + 1].startswith("-"):
        values[arg] = argv[i + 1]
        i += 2
        continue
    switches.append(arg)
    i += 1

if os.environ.get("MOCK_STRICT_ARGS") == "1":
    if values.get("-M") != "json-ocsf":
        print("prowler: error: -M must be json-ocsf", file=sys.stderr)
        sys.exit(7)
    for flag in ("-o", "-F"):
        if flag not in values:
            print("prowler: error: %s is required" % flag, file=sys.stderr)
            sys.exit(7)
    for flag in ("-b", "-z"):
        if flag not in switches:
            print("prowler: error: %s is required" % flag, file=sys.stderr)
            sys.exit(7)
    if "--severity" in values and values["--severity"] not in SEVERITIES:
        print("prowler: error: invalid --severity", file=sys.stderr)
        sys.exit(8)

print("Using provider: %s" % provider, flush=True)
print("* Getting findings...", flush=True)

if os.environ.get("MOCK_NO_REPORT") == "1":
    print("Findings available in ", flush=True)
    sys.exit(0)

report_dir = values.get("-o", "output")
report_name = values.get("-F", "prowler-output-demo")
os.makedirs(report_dir, exist_ok=True)
path = os.path.join(report_dir, report_name + ".ocsf.json")

if os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(path, "w", encoding="utf-8") as f:
        f.write('[{"metadata": {"event_code": "s3_bucket_public_access",')
    sys.exit(0)

if os.environ.get("MOCK_EMPTY") == "1":
    findings = []
else:
    findings = [
        {
            "message": "S3 bucket demo-bucket may be publicly accessible.",
            "metadata": {"event_code": "s3_bucket_public_access",
                         "product": {"name": "Prowler", "uid": "prowler"},
                         "profiles": ["cloud", "datetime"], "version": "1.3.0"},
            "severity_id": 4,
            "severity": "High",
            "status": "New",
            "status_code": "FAIL",
            "finding_info": {
                "title": "Check if S3 bucket has an ACL defined which allows "
                         "public READ",
                "uid": "prowler-aws-s3_bucket_public_access-123456789012-"
                       "eu-central-1-demo-bucket",
            },
            "resources": [{"type": "AwsS3Bucket", "name": "demo-bucket"}],
        },
        {
            "message": "Root account has hardware MFA enabled.",
            "metadata": {"event_code": "iam_root_account_hardware_mfa_enabled",
                         "product": {"name": "Prowler", "uid": "prowler"},
                         "profiles": ["cloud", "datetime"], "version": "1.3.0"},
            "severity_id": 1,
            "severity": "Informational",
            "status": "New",
            "status_code": "PASS",
            "finding_info": {
                "title": "Check if the root account has a hardware MFA enabled",
                "uid": "prowler-aws-iam_root_account_hardware_mfa_enabled-"
                       "123456789012",
            },
            "resources": [],
        },
    ]

with open(path, "w", encoding="utf-8") as f:
    json.dump(findings, f)

print("Findings available in %s" % path, flush=True)
print("Summary: AWS: %d successful, %d failed" % (1, 1), flush=True)