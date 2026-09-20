#!/usr/bin/env python3
"""jwt_decode — разбор JWT без верификации подписи (аудит, не auth)."""
import base64
import json
import sys
import time


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


def _b64url(part):
    pad = "=" * (-len(part) % 4)
    try:
        raw = base64.urlsafe_b64decode(part + pad)
    except Exception:
        return None
    try:
        obj = json.loads(raw.decode("utf-8"))
    except Exception:
        return None
    return obj if isinstance(obj, dict) else None


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    token = str(data.get("token") or "").strip()
    if not token:
        return fail("empty_input", "поле token пустое")
    parts = token.split(".")
    if len(parts) != 3:
        return fail("bad_token", "JWT: три части через точку")
    header, payload = _b64url(parts[0]), _b64url(parts[1])
    if header is None or payload is None:
        return fail("bad_token", "header/payload не base64url-JSON")

    exp = payload.get("exp")
    expired = isinstance(exp, (int, float)) and exp < time.time()
    return ok({"header": header, "payload": payload,
               "alg": str(header.get("alg") or ""),
               "expired": bool(expired),
               "signature_present": bool(parts[2])})


if __name__ == "__main__":
    sys.exit(main())
