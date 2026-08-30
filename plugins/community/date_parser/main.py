#!/usr/bin/env python3
"""date_parser — человекочитаемые даты → ISO 8601.

Поддерживает русские и английские форматы без внешних зависимостей.
stdin:  { "text": "12 марта 2024", "lang": "ru" }
stdout: { "status": "ok", "output": { "iso": "2024-03-12", "parsed": true, "format": "ru_d_month_y" } }
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


MONTHS_RU = {
    "января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6,
    "июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12,
}

MONTHS_EN = {
    "january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
    "july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
    "jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
    "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}


def parse_date(text: str, lang: str | None):
    text = text.strip().lower()
    if not text:
        return None

    # ISO уже
    if re.match(r"^\d{4}-\d{2}-\d{2}$", text):
        return text, "iso"

    # ru: 12.03.2024, 12/03/2024, 12-03-2024
    m = re.match(r"^(\d{1,2})[./-](\d{1,2})[./-](\d{4})$", text)
    if m:
        d, mon, y = map(int, m.groups())
        return f"{y:04d}-{mon:02d}-{d:02d}", "ru_numeric"

    # ru: 12 марта 2024
    m = re.match(r"^(\d{1,2})\s+(\w+)\s+(\d{4})$", text)
    if m and (lang is None or lang.startswith("ru")):
        d, mon_str, y = m.groups()
        mon = MONTHS_RU.get(mon_str)
        if mon:
            return f"{int(y):04d}-{mon:02d}-{int(d):02d}", "ru_d_month_y"

    # en: March 12, 2024
    m = re.match(r"^(\w+)\s+(\d{1,2}),?\s+(\d{4})$", text)
    if m and (lang is None or lang.startswith("en")):
        mon_str, d, y = m.groups()
        mon = MONTHS_EN.get(mon_str)
        if mon:
            return f"{int(y):04d}-{mon:02d}-{int(d):02d}", "en_month_d_y"

    # en: 03/12/2024 (month/day/year)
    m = re.match(r"^(\d{1,2})/(\d{1,2})/(\d{4})$", text)
    if m and (lang is None or lang.startswith("en")):
        mon, d, y = map(int, m.groups())
        return f"{y:04d}-{mon:02d}-{d:02d}", "en_md_y"

    return None


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}}, ensure_ascii=False))
        return 2

    text = data.get("text")
    if not isinstance(text, str):
        return fail("bad_input", "text должен быть строкой", retryable=False)

    lang = data.get("lang")
    if lang is not None and not isinstance(lang, str):
        return fail("bad_input", "lang должен быть строкой", retryable=False)

    result = parse_date(text, lang)
    if result is None:
        return fail("unparseable_date", f"не удалось распознать дату: {text!r}")

    iso, fmt = result
    return ok({"iso": iso, "parsed": True, "format": fmt})


if __name__ == "__main__":
    sys.exit(main())
