#!/usr/bin/env python3
"""email_extractor — вытаскивает email из текста, дедуп, домены."""
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


EMAIL_RE = re.compile(r"[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+")


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    text = str(data.get("text") or "")
    if not text.strip():
        return fail("empty_input", "поле text пустое")

    max_n = data.get("max_n")
    if max_n is None:
        max_n = 100
    try:
        max_n = int(max_n)
    except (TypeError, ValueError):
        return fail("bad_max_n", f"max_n должен быть числом, получено {max_n!r}")
    if max_n <= 0:
        return fail("bad_max_n", "max_n должен быть положительным")

    seen, emails = set(), []
    for m in EMAIL_RE.finditer(text):
        addr = m.group(0).strip().rstrip(".").lower()
        if addr not in seen:
            seen.add(addr)
            emails.append(addr)
        if len(emails) >= max_n:
            break
    if not emails:
        return fail("no_match", "email-адреса не найдены")

    domains = sorted({a.split("@", 1)[1] for a in emails})
    return ok({"emails": emails, "count": len(emails), "domains": domains})


if __name__ == "__main__":
    sys.exit(main())
