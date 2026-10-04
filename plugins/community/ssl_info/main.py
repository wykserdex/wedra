#!/usr/bin/env python3
"""ssl_info — снимает TLS-сертификат и разбирает сроки/издателя.

MOCK=1 — детерминированный ответ без сети (для контракт-тестов).
Проверка цепочки отключена намеренно: цель — аудит сертификата, включая
самоподписанные и просроченные сертификаты. DER разбирается через cryptography.
"""
import datetime
import json
import math
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


def fail_platform(code, message):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": False}},
                     ensure_ascii=False))
    return 2


def _integer_port(value):
    """Accept JSON integers and integral floats, but never truncate a port."""
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float) and math.isfinite(value) and value.is_integer():
        return int(value)
    return None


def _common_name(name, name_oid):
    attributes = name.get_attributes_for_oid(name_oid)
    return str(attributes[0].value) if attributes else ""


def _decode_certificate(der):
    # Import lazily so MOCK=1 and input validation do not need the optional
    # dependency; real certificate decoding does, and the manifest pins it.
    try:
        from cryptography import x509
        from cryptography.x509.oid import NameOID
    except ImportError as e:
        raise RuntimeError("не установлена зависимость cryptography") from e

    cert = x509.load_der_x509_certificate(der)
    not_after = cert.not_valid_after_utc
    subject_cn = _common_name(cert.subject, NameOID.COMMON_NAME)
    issuer_cn = _common_name(cert.issuer, NameOID.COMMON_NAME)

    # Совпадения CN недостаточно: проверяем, что сертификат действительно
    # подписан собственным ключом, а не только имеет одинаковые имена.
    try:
        cert.verify_directly_issued_by(cert)
        self_signed = True
    except Exception:
        self_signed = False

    return subject_cn, issuer_cn, not_after, self_signed


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

    raw_port = data.get("port")
    port = 443 if raw_port is None else _integer_port(raw_port)
    if port is None:
        return fail("bad_port", "port должен быть целым числом 1-65535")
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
            der = tls.getpeercert(binary_form=True)
    except Exception as e:
        return fail("tls_fail", f"handshake {host}:{port}: {e}",
                    retryable=True)
    finally:
        raw.close()

    if not der:
        return fail("bad_cert", "сервер не прислал сертификат")
    try:
        subj, issuer, not_after, self_signed = _decode_certificate(der)
    except RuntimeError as e:
        return fail_platform("missing_dependency", str(e))
    except Exception as e:
        return fail("bad_cert", f"не разобрать сертификат: {e}")

    now = datetime.datetime.now(datetime.timezone.utc)
    days_left = int((not_after - now).total_seconds() // 86400)
    return ok({
        "subject_cn": subj,
        "issuer_cn": issuer,
        "not_after": not_after.strftime("%Y-%m-%dT%H:%M:%S"),
        "days_left": days_left,
        "expired": now >= not_after,
        "self_signed": self_signed,
    })


if __name__ == "__main__":
    sys.exit(main())
