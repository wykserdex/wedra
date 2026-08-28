#!/usr/bin/env python3
"""disposable_checker — референс-плагин, протокол v0.1.

Офлайн-проверка домена по встроенному списку disposable-сервисов.
В проде список уедет в файл рядом с плагином (или в API-плагин с ключом);
для скелета важно: ноль сети, ноль зависимостей, детерминированный результат.
"""
import json
import sys

DISPOSABLE = {
    "mailinator.com", "tempmail.com", "temp-mail.org", "10minutemail.com",
    "guerrillamail.com", "yopmail.com", "trashmail.com", "trashmail.net",
    "sharklasers.com", "guerrillamailblock.com", "grr.la", "dispostable.com",
    "maildrop.cc", "fakeinbox.com", "getnada.com", "mohmal.com",
    "emailondeck.com", "mintemail.com", "mytemp.email", "tempmailo.com",
    "throwawaymail.com", "tmpmail.org", "spamgourmet.com", "mailnesia.com",
}


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        json.dump({"status": "error",
                   "error": {"code": "bad_input", "message": str(e)}}, sys.stdout)
        return 2

    email = str(data.get("email") or "").strip().lower()
    domain = email.split("@")[-1] if "@" in email else ""

    json.dump({"status": "ok",
               "output": {"disposable": domain in DISPOSABLE, "domain": domain}},
              sys.stdout)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
