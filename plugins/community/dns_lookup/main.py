#!/usr/bin/env python3
"""dns_lookup — резолв хоста через getaddrinfo (stdlib, без dnspython)."""
import concurrent.futures
import json
import socket
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


TIMEOUT = 10


def _resolve(host, fam):
    infos = socket.getaddrinfo(host, None, fam, socket.SOCK_STREAM)
    out, seen = [], set()
    for fi in infos:
        ip = fi[4][0]
        family = "6" if fi[0] == socket.AF_INET6 else "4"
        if (ip, family) not in seen:
            seen.add((ip, family))
            out.append({"ip": ip, "family": family})
    return out


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
    family = str(data.get("family") or "any").strip()
    if family == "4":
        fam = socket.AF_INET
    elif family == "6":
        fam = socket.AF_INET6
    elif family == "any":
        fam = socket.AF_UNSPEC
    else:
        return fail("bad_family", "family: 4|6|any")

    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=1) as ex:
            addrs = ex.submit(_resolve, host, fam).result(timeout=TIMEOUT)
    except (socket.gaierror, socket.herror) as e:
        return fail("dns_fail", f"не резолвится {host!r}: {e}")
    except Exception as e:
        return fail("dns_fail", f"{host!r}: {e}", retryable=True)
    if not addrs:
        return fail("no_records", f"пусто для {host!r}")
    return ok({"addresses": addrs, "count": len(addrs)})


if __name__ == "__main__":
    sys.exit(main())
