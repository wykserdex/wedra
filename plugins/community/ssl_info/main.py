#!/usr/bin/env python3
"""ssl_info — снимает TLS-сертификат и разбирает сроки/издателя.

MOCK=1 — детерминированный ответ без сети (для тестов/CI).
Проверка подписи отключена осознанно: цель — аудит, а не trust.
"""
import datetime
import json
import os
import socket
import ssl
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


def _cn(name):
    for part in name:
        for key, val in part:
            if key == "commonName":
                return val
    return ""


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    host = str(data.get("host") or "").strip()
    if not host:
        return fail("empty_input", "поле host пустое")
    if host.lower().rstrip(".").endswith((".invalid", ".test", ".example")):
        return fail("dns_fail", f"{host!r}: зарезервированный TLD, не резолвится")
    port = data.get("port")
    if port is None:
        port = 443
    try:
        port = int(port)
    except (TypeError, ValueError):
        return fail("bad_port", "port должен быть числом 1-65535")
    if not 1 <= port <= 65535:
        return fail("bad_port", "port вне 1-65535")

    if os.environ.get("MOCK") == "1":
        return ok({"subject_cn": "mock.example.com",
                   "issuer_cn": "Mock CA",
                   "not_after": "2030-01-01T00:00:00",
                   "days_left": 1000,
                   "expired": False,
                   "self_signed": False})

    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    try:
        raw = socket.create_connection((host, port), timeout=10)
    except socket.gaierror as e:
        return fail("dns_fail", f"не резолвится {host!r}: {e}")
    except Exception as e:
        return fail("network", f"{host}:{port}: {e}", retryable=True)
    try:
        with ctx.wrap_socket(raw, server_hostname=host) as tls:
            cert = tls.getpeercert()
    except Exception as e:
        return fail("tls_fail", f"handshake {host}:{port}: {e}",
                    retryable=True)
    finally:
        raw.close()

    subj = _cn(cert.get("subject", []))
    issuer = _cn(cert.get("issuer", []))
    try:
        na = datetime.datetime.strptime(cert["notAfter"], "%b %d %H:%M:%S %Y %Z")
    except Exception:
        return fail("bad_cert", "не разобрать notAfter")
    now = datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None)
    days_left = (na - now).days
    return ok({
        "subject_cn": subj,
        "issuer_cn": issuer,
        "not_after": na.strftime("%Y-%m-%dT%H:%M:%S"),
        "days_left": days_left,
        "expired": days_left < 0,
        "self_signed": bool(subj) and subj == issuer,
    })


if __name__ == "__main__":
    sys.exit(main())
