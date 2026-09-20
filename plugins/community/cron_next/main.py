#!/usr/bin/env python3
"""cron_next — ближайшее срабатывание cron (мин час дом мес дов) от момента.

Поддерживает *, */n, a-b, a-b/n, списки, имена mon..sun / jan..dec.
День-месяца vs день-недели — классическая OR-семантика cron.
"""
import datetime
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


MONTHS = {"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
          "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
DOWS = {"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5,
        "sat": 6}


def _parse_field(text, lo, hi, names=None):
    vals = set()
    text = text.strip().lower()
    if not text:
        raise ValueError("пустое поле")
    for chunk in text.split(","):
        chunk = chunk.strip()
        step = 1
        if "/" in chunk:
            chunk, step_s = chunk.split("/", 1)
            try:
                step = int(step_s)
            except ValueError:
                raise ValueError(f"плохой шаг {step_s!r}")
            if step < 1:
                raise ValueError("шаг < 1")
        if chunk in ("*", ""):
            start, end = lo, hi
        elif "-" in chunk:
            a_s, b_s = chunk.split("-", 1)
            start, end = _num(a_s, names), _num(b_s, names)
        else:
            start = end = _num(chunk, names)
        if not (lo <= start <= hi and lo <= end <= hi) or start > end:
            raise ValueError(f"диапазон {chunk!r} вне {lo}-{hi}")
        vals.update(range(start, end + 1, step))
    return vals, text == "*"


def _num(token, names):
    token = token.strip()
    if names and token in names:
        return names[token]
    try:
        return int(token)
    except ValueError:
        raise ValueError(f"плохое значение {token!r}")


def _cron_match(dt, minute, hour, dom, month, dow, dom_star, dow_star):
    # cronDow: 0 и 7 = воскресенье
    cron_dow = (dt.isoweekday()) % 7
    if dt.minute not in minute:
        return False
    if dt.hour not in hour:
        return False
    if dt.month not in month:
        return False
    if dom_star and dow_star:
        return True
    if dom_star:
        return cron_dow in dow
    if dow_star:
        return dt.day in dom
    return dt.day in dom or cron_dow in dow


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    expr = str(data.get("expr") or "").strip()
    fields = expr.split()
    if len(fields) != 5:
        return fail("bad_expr", "нужно 5 полей: мин час дом мес дов")
    try:
        minute, _ = _parse_field(fields[0], 0, 59)
        hour, _ = _parse_field(fields[1], 0, 23)
        dom, dom_star = _parse_field(fields[2], 1, 31)
        month, _ = _parse_field(fields[3], 1, 12, MONTHS)
        dow_raw = re.sub(r"(?<![0-9])7(?![0-9])", "0",
                         fields[4].strip().lower())
        dow, dow_star = _parse_field(dow_raw, 0, 6, DOWS)
    except ValueError as e:
        return fail("bad_expr", str(e))

    from_s = str(data.get("from") or "").strip()
    if from_s:
        try:
            cur = datetime.datetime.fromisoformat(from_s)
        except ValueError:
            return fail("bad_from", "from: ISO 8601")
    else:
        cur = datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None)
    cur = cur.replace(second=0, microsecond=0) + datetime.timedelta(minutes=1)

    for _ in range(525600 + 525600):
        try:
            ok_day = _cron_match(cur, minute, hour, dom, month, dow,
                                 dom_star, dow_star)
        except ValueError:
            ok_day = False
        if ok_day:
            return ok({"next": cur.strftime("%Y-%m-%dT%H:%M:%S")})
        cur += datetime.timedelta(minutes=1)
    return fail("no_occurrence", "нет срабатываний за 2 года")


if __name__ == "__main__":
    sys.exit(main())
