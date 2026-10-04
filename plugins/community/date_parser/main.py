#!/usr/bin/env python3
"""date_parser — человекочитаемые даты → проверенный ISO 8601.

Неоднозначная числовая дата со слешами требует явного lang.
stdin:  { "text": "12 марта 2024", "lang": "ru" }
stdout: { "status": "ok", "output": { "iso": "2024-03-12", "parsed": true, "format": "ru_d_month_y" } }
"""
import datetime
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


def _validated_result(year, month, day, fmt):
    try:
        parsed = datetime.date(year, month, day)
    except ValueError:
        return None
    return parsed.isoformat(), fmt


def _english_order(lang):
    """Return True for month-first English, False for day-first English."""
    if not lang:
        return None
    normalized = lang.lower().replace("_", "-")
    if not normalized.startswith("en"):
        return None
    # English is otherwise treated as US-style; common DMY regions are explicit.
    region = normalized.split("-", 1)[1].upper() if "-" in normalized else ""
    return region not in {"GB", "AU", "NZ", "IE"}


def parse_date(text: str, lang: str | None):
    text = text.strip().lower()
    if not text:
        return None

    # ISO: require a real calendar date, not merely YYYY-MM-DD-shaped text.
    if re.fullmatch(r"\d{4}-\d{2}-\d{2}", text):
        year, month, day = map(int, text.split("-"))
        return _validated_result(year, month, day, "iso")

    numeric = re.fullmatch(r"(\d{1,2})([./-])(\d{1,2})\2(\d{4})", text)
    if numeric:
        first, separator, second, year = numeric.groups()
        first, second, year = int(first), int(second), int(year)
        month_first = _english_order(lang)

        if separator == "/" and lang is None:
            # Infer only when one interpretation is impossible. For 03/12,
            # silently choosing a locale would turn a valid date into the wrong one.
            if first <= 12 and second <= 12:
                return None
            if first > 12 and second <= 12:
                month_first = False
            elif second > 12 and first <= 12:
                month_first = True
            else:
                return None
        elif separator == "/" and lang is not None:
            lang_key = lang.lower().replace("_", "-")
            if lang_key.startswith("ru"):
                month_first = False
            elif lang_key.startswith("en"):
                month_first = _english_order(lang)
            else:
                return None
        elif separator in ".-":
            if lang is not None and lang.lower().startswith("en"):
                month_first = _english_order(lang)
            elif lang is None or lang.lower().startswith("ru"):
                month_first = False
            else:
                return None

        if month_first is None:
            return None
        if month_first:
            fmt = "en_md_y" if separator == "/" else "en_numeric"
            return _validated_result(year, first, second, fmt)
        fmt = "en_dmy_y" if lang and lang.lower().startswith("en") else "ru_numeric"
        return _validated_result(year, second, first, fmt)

    # Russian: 12 марта 2024
    match = re.fullmatch(r"(\d{1,2})\s+(\w+)\s+(\d{4})", text)
    if match and (lang is None or lang.lower().startswith("ru")):
        day, month_name, year = match.groups()
        month = MONTHS_RU.get(month_name)
        if month:
            return _validated_result(int(year), month, int(day), "ru_d_month_y")

    # English: March 12, 2024
    match = re.fullmatch(r"(\w+)\s+(\d{1,2}),?\s+(\d{4})", text)
    if match and (lang is None or lang.lower().startswith("en")):
        month_name, day, year = match.groups()
        month = MONTHS_EN.get(month_name)
        if month:
            return _validated_result(int(year), month, int(day), "en_month_d_y")

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
