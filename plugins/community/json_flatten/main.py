#!/usr/bin/env python3
"""json_flatten — вложенный JSON → плоский map.

Разделитель входит в буквальный ключ как есть; если разные пути дают один
плоский ключ, плагин возвращает key_collision вместо тихой потери значения.
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
        max_depth = depth if not obj else 0
        for key, value in obj.items():
            new_key = f"{prefix}{sep}{key}" if prefix else str(key)
            _, child_depth = flatten(value, sep, new_key, acc, depth + 1)
            max_depth = max(max_depth, child_depth)
        return acc, max_depth
    if isinstance(obj, list):
        max_depth = depth if not obj else 0
        for index, value in enumerate(obj):
            new_key = f"{prefix}{sep}{index}" if prefix else str(index)
            _, child_depth = flatten(value, sep, new_key, acc, depth + 1)
            max_depth = max(max_depth, child_depth)
        return acc, max_depth

    if prefix in acc:
        raise ValueError(f"разные пути дают плоский ключ {prefix!r}")
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
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": "data должен быть объектом",
            "retryable": False}}, ensure_ascii=False))
        return 2

    sep = data.get("separator", ".")
    if not isinstance(sep, str) or len(sep) != 1:
        return fail("bad_input", "separator должен быть одним символом")

    try:
        flat, max_depth = flatten(obj, sep)
    except ValueError as e:
        return fail("key_collision", str(e))

    return ok({"flat": flat, "key_count": len(flat), "max_depth": max_depth})


if __name__ == "__main__":
    sys.exit(main())
