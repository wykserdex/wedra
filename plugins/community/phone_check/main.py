#!/usr/bin/env python3
"""phone_normalizer — любой формат → E.164.

Лёгкая офлайн-нормализация без libphonenumber.
stdin:  { "phone": "+375 (29) 123-45-67" }
stdout: { "status": "ok", "output": { "normalized": "+375291234567", "valid": true, "country": "BY" } }
"""
import json
import re
import sys


def fail(code, message, retryable=False):
    print(json.dumps({
        "status": "error",
        "error": {"code": code, "message": message, "retryable": retryable}
    }, ensure_ascii=False))
    return 1


def ok(payload):
    print(json.dumps({"status": "ok", "output": payload}, ensure_ascii=False))
    return 0


# Простая карта мобильных кодов → страна (для демо, расширяемая).
COUNTRY_BY_CODE = {
    "375": "BY", "7": "RU", "380": "UA", "48": "PL", "49": "DE",
    "44": "GB", "1": "US", "33": "FR", "39": "IT", "34": "ES",
    "90": "TR", "86": "CN", "91": "IN", "81": "JP", "82": "KR",
}


def normalize(phone: str, default_country: str | None):
    raw = re.sub(r"[^\d+]", "", phone.strip())
    if not raw:
        return None

    digits = raw.lstrip("+")
    if raw.startswith("+"):
        # уже с +
        cc = next((c for c in sorted(COUNTRY_BY_CODE, key=len, reverse=True) if digits.startswith(c)), None)
    else:
        # без + — пробуем определить по длине/коду или подставить default_country
        cc = None
        if default_country:
            cc = next((c for c, country in COUNTRY_BY_CODE.items() if country.upper() == default_country.upper()), None)
        if not cc:
            cc = next((c for c in sorted(COUNTRY_BY_CODE, key=len, reverse=True) if digits.startswith(c)), None)
        digits = cc + digits[len(cc):] if cc else digits

    if len(digits) < 7 or len(digits) > 15:
        return None

    country = COUNTRY_BY_CODE.get(cc) if cc else None
    return f"+{digits}", bool(country), country


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}}, ensure_ascii=False))
        return 2

    phone = data.get("phone")
    if not isinstance(phone, str):
        # guard типа → платформенная ошибка (exit 2): виноват вызывающий
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": "phone должен быть строкой",
            "retryable": False}}, ensure_ascii=False))
        return 2

    default_country = data.get("default_country")
    if default_country is not None and not isinstance(default_country, str):
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": "default_country должен быть строкой",
            "retryable": False}}, ensure_ascii=False))
        return 2

    result = normalize(phone, default_country)
    if result is None:
        return fail("invalid_phone", f"не удалось нормализовать номер: {phone!r}")

    normalized, valid, country = result
    return ok({"normalized": normalized, "valid": valid, "country": country or ""})


if __name__ == "__main__":
    sys.exit(main())
