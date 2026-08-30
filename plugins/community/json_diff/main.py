#!/usr/bin/env python3
"""json_diff — сравнивает два плоских JSON-объекта: added/removed/changed ключи.

Полезно для сравнения снапшотов конфигов, API-ответов, состояний "до/после".

Протокол (PROTOCOL.md):
  stdin  ← JSON {"before": {...}, "after": {...}}
  stdout → status: ok/error
"""
import json
import sys


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return 1


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    before = data.get("before")
    after = data.get("after")
    if not isinstance(before, dict) or not isinstance(after, dict):
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input",
            "message": "before и after должны быть объектами (type: object)",
            "retryable": False}}, ensure_ascii=False))
        return 2

    if not before and not after:
        return fail("empty_input", "before и after оба пусты — нечего сравнивать")

    before_keys = set(before.keys())
    after_keys = set(after.keys())

    added = sorted(after_keys - before_keys)
    removed = sorted(before_keys - after_keys)
    changed = sorted(
        k for k in (before_keys & after_keys) if before[k] != after[k]
    )
    unchanged = sorted(
        k for k in (before_keys & after_keys) if before[k] == after[k]
    )

    return ok({
        "added": added,
        "removed": removed,
        "changed": changed,
        "unchanged_count": len(unchanged),
        "has_diff": bool(added or removed or changed),
    })


if __name__ == "__main__":
    sys.exit(main())
