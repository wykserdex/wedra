#!/usr/bin/env python3
"""domain_parser — офлайн-разбор URL/хоста на компоненты + punycode."""
import ipaddress
import json
import sys
from urllib.parse import urlsplit


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


def _is_ip(host):
    try:
        ipaddress.ip_address(host)
        return True
    except ValueError:
        return False


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    raw = str(data.get("target") or "").strip()
    if not raw:
        return fail("empty_input", "поле target пустое")

    text = raw if "://" in raw else "//" + raw
    try:
        parts = urlsplit(text)
    except Exception as e:
        return fail("bad_target", f"не разобрать: {e}")
    host = (parts.hostname or "").strip()
    if not host:
        return fail("bad_target", f"нет хоста в {raw!r}")

    try:
        ascii_host = host.encode("idna").decode("ascii")
    except Exception:
        ascii_host = host
    try:
        uni_host = ascii_host.encode("ascii").decode("idna")
    except Exception:
        uni_host = ascii_host

    return ok({
        "scheme": parts.scheme or "",
        "host": uni_host,
        "ascii_host": ascii_host,
        "port": parts.port or 0,
        "path": parts.path or "",
        "is_ip": _is_ip(host.strip("[]")),
    })


if __name__ == "__main__":
    sys.exit(main())
