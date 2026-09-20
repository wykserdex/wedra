#!/usr/bin/env python3
"""regex_extract — находит все совпадения паттерна в тексте (с лимитом)."""
import json
import re
import sys


try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


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

    pattern = str(data.get("pattern") or "")
    if not pattern:
        return fail("empty_input", "поле pattern пустое")
    text = str(data.get("text") or "")
    if not text:
        return fail("empty_input", "поле text пустое")
    try:
        rx = re.compile(pattern)
    except re.error as e:
        return fail("bad_pattern", f"невалидный regex: {e}")

    max_n = data.get("max_n")
    if max_n is None:
        max_n = 50
    try:
        max_n = int(max_n)
    except (TypeError, ValueError):
        return fail("bad_max_n", "max_n должен быть числом")
    if not 1 <= max_n <= 1000:
        return fail("bad_max_n", "max_n вне 1-1000")

    matches = []
    for m in rx.finditer(text):
        matches.append(m.group(1) if m.lastindex else m.group(0))
        if len(matches) >= max_n:
            break
    if not matches:
        return fail("no_match", "совпадений нет")
    return ok({"matches": matches, "count": len(matches)})


if __name__ == "__main__":
    sys.exit(main())
