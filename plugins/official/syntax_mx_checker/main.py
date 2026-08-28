#!/usr/bin/env python3
"""syntax_mx_checker — референс-плагин, протокол v0.1 (см. PROTOCOL.md).

stdin:  {"email": "..."}
stdout: {"status": "ok", "output": {"syntax": true, "mx": [...]}}
        {"status": "error", "error": {"code": ..., "message": ...}}  + exit 1
"""
import json
import re
import sys

EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")


def fail(code, message, exit_code=1):
    json.dump({"status": "error", "error": {"code": code, "message": message}}, sys.stdout)
    print()
    return exit_code


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)

    email = str(data.get("email") or "").strip()
    if not EMAIL_RE.match(email):
        return fail("bad_syntax", f"{email!r} не похож на email")

    mx = []
    try:
        import dns.resolver

        domain = email.split("@")[1]
        try:
            mx = sorted(
                str(r.exchange).rstrip(".")
                for r in dns.resolver.resolve(domain, "MX", lifetime=5.0)
            )
        except Exception as e:
            # DNS недоступен/домен без MX — не фейлим шаг, честно логируем в stderr
            print(f"mx lookup failed for {domain}: {e}", file=sys.stderr)
    except ImportError:
        print("dnspython не установлен — MX-проверка пропущена", file=sys.stderr)

    json.dump({"status": "ok", "output": {"syntax": True, "mx": mx}}, sys.stdout)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
