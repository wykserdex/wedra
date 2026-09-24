#!/usr/bin/env python3
"""Фикстура: падает retryable-ошибкой первые 2 вызова, на 3-й отдаёт ok.

Состояние — файл _counter рядом с main.py; тесты удаляют его перед прогоном.
"""
import json
import os
import sys

counter_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "_counter")

try:
    with open(counter_path) as f:
        n = int(f.read().strip())
except Exception:
    n = 0
n += 1
with open(counter_path, "w") as f:
    f.write(str(n))

if n <= 2:
    print(json.dumps({"status": "error", "error": {
        "code": "rate_limit", "message": "429 slow down", "retryable": True}}))
    sys.exit(1)

print(json.dumps({"status": "ok", "output": {"result": "ok"}}))
