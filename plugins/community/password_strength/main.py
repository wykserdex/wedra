#!/usr/bin/env python3
"""password_strength — скоринг 0-4 без хранения пароля в открытом виде в логах."""
import json
import math
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


WEAK = {"123456", "12345678", "password", "qwerty", "qwerty123", "111111",
        "abc123", "123123", "admin", "letmein", "welcome", "monkey",
        "dragon", "football", "iloveyou", "000000", "пароль", "йцукен"}


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    pw = str(data.get("password") or "")
    if not pw:
        return fail("empty_input", "поле password пустое")

    feedback = []
    low = pw.lower()
    if low in WEAK or (len(pw) <= 6 and pw.isdigit()):
        return ok({"score": 0, "entropy_bits": 0,
                   "feedback": ["пароль из стоп-листа самых частых"]})

    classes = sum((any(c.islower() for c in pw),
                   any(c.isupper() for c in pw),
                   any(c.isdigit() for c in pw),
                   any(not c.isalnum() for c in pw)))
    pool = (26 * any(c.islower() for c in pw)
            + 26 * any(c.isupper() for c in pw)
            + 10 * any(c.isdigit() for c in pw)
            + 32 * any(not c.isalnum() and not c.isspace() for c in pw)
            + 5 * any(c.isspace() for c in pw))
    entropy = round(len(pw) * math.log2(pool or 1), 1)

    score = sum((len(pw) >= 8, len(pw) >= 12, classes >= 3, entropy >= 50))
    if len(pw) < 8:
        feedback.append("короче 8 символов")
    if classes < 3:
        feedback.append("мало классов символов (нужны 3+: aA1#)")
    if entropy < 50:
        feedback.append("низкая энтропия — длиннее и разнообразнее")
    if not feedback:
        feedback.append("годный пароль")

    return ok({"score": score, "entropy_bits": entropy, "feedback": feedback})


if __name__ == "__main__":
    sys.exit(main())
