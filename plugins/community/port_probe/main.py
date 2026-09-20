#!/usr/bin/env python3
"""port_probe — TCP connect + замер задержки + попытка снять баннер.

MOCK=1 — детерминированный ответ без сети (для тестов/CI).
"""
import json
import os
import socket
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
    try:
        port = int(data.get("port"))
    except (TypeError, ValueError):
        return fail("bad_port", f"port должен быть числом 1-65535")
    if not 1 <= port <= 65535:
        return fail("bad_port", "port вне 1-65535")
    try:
        timeout = float(data.get("timeout_s") or 5)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout_s должен быть числом")
    if timeout <= 0 or timeout > 60:
        return fail("bad_timeout", "timeout_s вне (0, 60]")

    if os.environ.get("MOCK") == "1":
        return ok({"open": True, "latency_ms": 3, "banner": "220 mock ESMTP"})

    start = time.monotonic()
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.settimeout(timeout)
        rc = sock.connect_ex((host, port))
        latency = int((time.monotonic() - start) * 1000)
        if rc != 0:
            return fail("conn_refused",
                        f"{host}:{port} закрыт/недоступен (код {rc})")
        banner = ""
        try:
            sock.settimeout(min(timeout, 2))
            chunk = sock.recv(256)
            banner = chunk.decode("utf-8", "replace").strip()
        except Exception:
            banner = ""
        return ok({"open": True, "latency_ms": latency, "banner": banner})
    except socket.gaierror as e:
        return fail("dns_fail", f"не резолвится {host!r}: {e}")
    except Exception as e:
        return fail("network", str(e), retryable=True)
    finally:
        sock.close()


if __name__ == "__main__":
    sys.exit(main())
