#!/usr/bin/env python3
"""Mock CloudMapper CLI для контракт-тестов (без сети и без пакета).

Имитирует два вызова:
  collect --config <cfg> --account <name> [--profile P] [--regions CSV]
  prepare --config <cfg> --account <name> [--regions CSV]
collect только проверяет конфиг и печатает лог (данные «собирает» в
account-data/, но нам они не нужны), prepare пишет web/data.json — список
элементов cytoscape в формате shared/nodes.py. Режимы env: MOCK_SLEEP=N (тест
wall_timeout), MOCK_NO_REPORT=1 (тест no_report), MOCK_FAIL=1 (ненулевой код →
тест tool_failed), MOCK_BAD_REPORT=1 (битый JSON → тест bad_report),
MOCK_EMPTY=1 (пустой граф), MOCK_STRICT_CONFIG=1 (конфиг обязан быть на диске и
содержать аккаунт из --account), MOCK_STRICT_ARGS=1 (collect обязан получить
--config/--account, prepare — те же флаги).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))

if os.environ.get("MOCK_FAIL") == "1":
    print("ERROR: sts.get_caller_identity failed with InvalidClientTokenId.",
          file=sys.stderr)
    sys.exit(2)

argv = sys.argv[1:]
if argv and not argv[0].startswith("-"):
    command = argv[0]
    rest = argv[1:]
else:
    command = ""
    rest = argv

flags = {}
i = 0
while i < len(rest):
    if rest[i].startswith("--") and i + 1 < len(rest):
        flags[rest[i]] = rest[i + 1]
        i += 2
        continue
    i += 1

config = flags.get("--config", "")
account = flags.get("--account", "")

if not command:
    print("ERROR: usage: cloudmapper.py {collect,prepare} --account <name>",
          file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_STRICT_CONFIG") == "1":
    if not config or not os.path.isfile(config):
        print("ERROR: Unable to load config file", file=sys.stderr)
        sys.exit(4)
    with open(config, encoding="utf-8") as f:
        parsed = json.load(f)
    names = [a.get("name") for a in parsed.get("accounts", [])]
    if account not in names:
        print('ERROR: Account named "{}" not found in {}'.format(account, config),
              file=sys.stderr)
        sys.exit(5)

if command == "collect":
    if os.environ.get("MOCK_STRICT_ARGS") == "1" and (not config or not account):
        print("ERROR: --config and --account are required", file=sys.stderr)
        sys.exit(6)
    print("* Getting region names", flush=True)
    print("Summary: {} APIs called. 0 errors".format(14), flush=True)
    print("* Collecting data for account {}".format(account), file=sys.stderr)
    os.makedirs("account-data/{}".format(account), exist_ok=True)
    sys.exit(0)

if command != "prepare":
    print("ERROR: unknown command {}".format(command), file=sys.stderr)
    sys.exit(1)

if os.environ.get("MOCK_STRICT_ARGS") == "1" and (not config or not account):
    print("ERROR: --config and --account are required", file=sys.stderr)
    sys.exit(6)

print("WARNING: This functionality is no longer maintained", file=sys.stderr)
print("Building data for account {}".format(account), file=sys.stderr)

if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

os.makedirs("web", exist_ok=True)
path = os.path.join("web", "data.json")

if os.environ.get("MOCK_EMPTY") == "1":
    graph = []
elif os.environ.get("MOCK_BAD_REPORT") == "1":
    with open(path, "w", encoding="utf-8") as f:
        f.write('[{"data": {"id": "arn:aws:::123456789012:', )
    sys.exit(0)
else:
    account_arn = "arn:aws:::123456789012:"
    graph = [
        {"data": {"id": account_arn, "name": account, "type": "account",
                  "local_id": "123456789012", "node_data": {"id": "123456789012"}}},
        {"data": {"id": "arn:aws::us-east-1:123456789012:", "name": "us-east-1",
                  "type": "region", "local_id": "us-east-1",
                  "parent": account_arn, "node_data": {"RegionName": "us-east-1"}}},
        {"data": {"id": "arn:aws:ec2:us-east-1:123456789012:instance/i-0demo",
                  "name": "i-0demo", "type": "ec2", "local_id": "i-0demo",
                  "parent": "arn:aws::us-east-1:123456789012:",
                  "node_data": {"InstanceId": "i-0demo"}}},
        {"data": {"source": "0.0.0.0/0",
                  "target": "arn:aws:ec2:us-east-1:123456789012:instance/i-0demo",
                  "type": "edge", "node_data": [{"GroupId": "sg-0demo"}]}},
    ]

with open(path, "w", encoding="utf-8") as f:
    json.dump(graph, f, indent=4)

print("- 1 nodes built in region us-east-1", file=sys.stderr)
print("- 1 connections built", file=sys.stderr)