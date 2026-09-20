#!/usr/bin/env python3
"""ip_intel — офлайн-разбор IP: версия, классы, нормализация, int-представление."""
import ipaddress
import json
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

    raw = str(data.get("ip") or "").strip()
    if not raw:
        return fail("empty_input", "поле ip пустое")
    try:
        addr = ipaddress.ip_address(raw)
    except ValueError:
        return fail("bad_ip", f"не IP-адрес: {raw!r}")

    return ok({
        "normalized": str(addr),
        "version": addr.version,
        "is_private": addr.is_private,
        "is_loopback": addr.is_loopback,
        "is_multicast": addr.is_multicast,
        "is_reserved": addr.is_reserved,
        "int_value": str(int(addr)),
    })


if __name__ == "__main__":
    sys.exit(main())
