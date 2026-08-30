#!/usr/bin/env python3
"""json_flatten — вложенный JSON → плоский map.

stdin:  { "data": { "user": { "name": "Anna", "age": 30 } }, "separator": "." }
stdout: { "status": "ok", "output": { "flat": { "user.name": "Anna", "user.age": 30 }, "key_count": 2, "max_depth": 2 } }
"""
import json
import sys


def fail(code, message, retryable=False):
    print(json.dumps({
        "status": "error",
        "error": {"code": code, "message": message, "retryable": retryable}
    }, ensure_ascii=False))
    return 1


def ok(payload):
    print(json.dumps({"status": "ok", "output": payload}, ensure_ascii=False))
    return 0


def flatten(obj, sep=".", prefix="", acc=None, depth=0):
    if acc is None:
        acc = {}
    if isinstance(obj, dict):
        for k, v in obj.items():
            new_key = f"{prefix}{sep}{k}" if prefix else k
            flatten(v, sep, new_key, acc, depth + 1)
    elif isinstance(obj, list):
        for i, v in enumerate(obj):
            new_key = f"{prefix}{sep}{i}" if prefix else str(i)
            flatten(v, sep, new_key, acc, depth + 1)
    else:
        acc[prefix] = obj
    return acc, depth


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}}, ensure_ascii=False))
        return 2

    obj = data.get("data")
    if not isinstance(obj, dict):
        # guard типа → платформенная ошибка (exit 2): виноват вызывающий
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": "data должен быть объектом",
            "retryable": False}}, ensure_ascii=False))
        return 2

    sep = data.get("separator", ".")
    if not isinstance(sep, str) or len(sep) != 1:
        return fail("bad_input", "separator должен быть одним символом")

    flat, _ = flatten(obj, sep)
    max_depth = 0
    for key in flat:
        max_depth = max(max_depth, key.count(sep) + 1)

    return ok({"flat": flat, "key_count": len(flat), "max_depth": max_depth})


if __name__ == "__main__":
    sys.exit(main())
