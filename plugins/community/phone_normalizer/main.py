#!/usr/bin/env python3
"""phone_normalizer — нормализация списка телефонов в E.164 (батч, без сети).

Один грязный номер в списке — не повод ронять шаг: такие номера попадают
в invalid с кодом ошибки, шаг завершается ok. Доменная ошибка — только
пустой список. Битый JSON / не-массив — платформенная ошибка (exit 2).
"""
import json
import re
import sys

# Код страны -> страна (код 7 делят RU и KZ, код 1 — US/CA)
CODE_COUNTRY = {
    "7": "RU/KZ", "1": "US/CA", "49": "DE", "44": "GB", "33": "FR",
    "39": "IT", "34": "ES", "31": "NL", "48": "PL", "380": "UA",
    "375": "BY", "86": "CN", "91": "IN", "55": "BR", "81": "JP",
    "90": "TR", "996": "KG", "998": "UZ", "373": "MD",
}

# Дефолтная страна -> код (для номеров без '+')
COUNTRY_CODE = {
    "RU": "7", "KZ": "7", "US": "1", "CA": "1", "DE": "49", "GB": "44",
    "FR": "33", "IT": "39", "ES": "34", "NL": "31", "PL": "48",
    "UA": "380", "BY": "375", "CN": "86", "IN": "91", "BR": "55",
    "JP": "81", "TR": "90", "KG": "996", "UZ": "998", "MD": "373",
}


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return 1


def platform_bad_input(message):
    print(json.dumps({"status": "error", "error": {
        "code": "bad_input", "message": message, "retryable": False}},
        ensure_ascii=False))
    return 2


def invalid_entry(raw, code, message):
    return {"input": raw, "valid": False, "error": code, "message": message}


def normalize_one(raw, default_country):
    if not isinstance(raw, str):
        return invalid_entry(str(raw), "bad_entry", "номер должен быть строкой")

    p = raw.strip()
    has_plus = p.startswith("+")
    digits = re.sub(r"\D", "", p)

    if not digits:
        return invalid_entry(raw, "no_digits", "в строке нет цифр")

    if has_plus:
        cc = None
        for size in (3, 2, 1):
            candidate = digits[:size]
            if candidate in CODE_COUNTRY:
                cc = candidate
                break
        if cc is None:
            return invalid_entry(raw, "unknown_country",
                                 "код страны %s не распознан" % digits[:3])
        national = digits[len(cc):]
    else:
        if default_country not in COUNTRY_CODE:
            return invalid_entry(raw, "no_country_code",
                                 "нет кода страны и не задан default_country")
        cc = COUNTRY_CODE[default_country]
        national = digits
        # транк: в RU городские номера начинаются с 8 вместо +7
        if (default_country == "RU" and national.startswith("8")
                and len(national) == 11):
            national = national[1:]

    if len(national) < 4:
        return invalid_entry(raw, "bad_length",
                             "национальная часть короче 4 цифр")
    if len(cc) + len(national) > 15:
        return invalid_entry(raw, "bad_length",
                             "номер длиннее 15 цифр E.164")

    return {
        "input": raw,
        "valid": True,
        "e164": "+" + cc + national,
        "country": CODE_COUNTRY.get(cc, default_country),
        "national": national,
    }


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:  # noqa: BLE001
        return platform_bad_input("stdin не является JSON: %s" % e)

    if not isinstance(data, dict) or not isinstance(data.get("phones"), list):
        return platform_bad_input("ожидался объект {phones: [..]}")

    phones = data["phones"]
    if not phones:
        return fail("empty_input", "список phones пуст")

    default = data.get("default_country") or ""
    if not isinstance(default, str):
        return fail("bad_default_country", "default_country должен быть строкой")

    results = [normalize_one(raw, default) for raw in phones]
    valid = [r for r in results if r.get("valid")]
    invalid = [r for r in results if not r.get("valid")]

    return ok({
        "results": results,
        "valid": valid,
        "invalid": invalid,
        "valid_count": len(valid),
        "invalid_count": len(invalid),
    })


if __name__ == "__main__":
    sys.exit(main())
