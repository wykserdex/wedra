#!/usr/bin/env python3
"""hash_tool — identify (md5/sha*/bcrypt/argon2/uuid) и compute (hashlib)."""
import hashlib
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


HEX = r"[0-9a-fA-F]"
UUID_RE = re.compile(
    r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-"
    r"[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")


def identify(value):
    guesses = []
    if UUID_RE.match(value):
        guesses.append({"algo": "uuid", "bits": 122})
    if re.fullmatch(HEX + r"{32}", value):
        guesses.append({"algo": "md5", "bits": 128})
    if re.fullmatch(HEX + r"{40}", value):
        guesses.append({"algo": "sha1", "bits": 160})
    if re.fullmatch(HEX + r"{56}", value):
        guesses.append({"algo": "sha224", "bits": 224})
    if re.fullmatch(HEX + r"{64}", value):
        guesses.append({"algo": "sha256", "bits": 256})
    if re.fullmatch(HEX + r"{96}", value):
        guesses.append({"algo": "sha384", "bits": 384})
    if re.fullmatch(HEX + r"{128}", value):
        guesses.append({"algo": "sha512", "bits": 512})
    if re.fullmatch(r"\$2[aby]\$\d{2}\$.{53}", value):
        guesses.append({"algo": "bcrypt", "bits": 184})
    if value.startswith("$argon2"):
        guesses.append({"algo": "argon2", "bits": 0})
    return guesses


COMPUTE = {"md5": "md5", "sha1": "sha1", "sha224": "sha224",
           "sha256": "sha256", "sha384": "sha384", "sha512": "sha512"}


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    mode = str(data.get("mode") or "").strip().lower()
    value = str(data.get("value") or "")
    if mode not in ("identify", "compute"):
        return fail("bad_mode", "mode: identify|compute")
    if not value:
        return fail("empty_input", "поле value пустое")

    if mode == "identify":
        return ok({"guesses": identify(value.strip()), "digest": ""})

    algo = str(data.get("algo") or "").strip().lower()
    if algo not in COMPUTE:
        return fail("bad_algo", f"algo: {sorted(COMPUTE)}")
    digest = hashlib.new(COMPUTE[algo], value.encode("utf-8")).hexdigest()
    return ok({"guesses": [], "digest": digest})


if __name__ == "__main__":
    sys.exit(main())
