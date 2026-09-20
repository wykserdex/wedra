#!/usr/bin/env python3
"""csv_stats — описательные статистики числовой колонки CSV (из текста)."""
import csv
import io
import json
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


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    text = str(data.get("csv_text") or "")
    if not text.strip():
        return fail("empty_input", "поле csv_text пустое")
    column = str(data.get("column") or "").strip()
    if not column:
        return fail("empty_input", "поле column пустое")

    try:
        rows = list(csv.DictReader(io.StringIO(text)))
    except Exception as e:
        return fail("bad_csv", f"не CSV: {e}")
    if not rows or not rows[0]:
        return fail("bad_csv", "пустой CSV или нет заголовка")
    if column not in rows[0]:
        return fail("no_column", f"колонки {column!r} нет, есть: "
                                 f"{sorted(rows[0])}")

    vals, skipped = [], 0
    for r in rows:
        cell = (r.get(column) or "").strip().replace(",", ".")
        try:
            vals.append(float(cell))
        except ValueError:
            skipped += 1
    if not vals:
        return fail("no_numeric", f"в колонке {column!r} нет чисел")

    total = sum(vals)
    return ok({"count": len(vals), "mean": total / len(vals),
               "min": min(vals), "max": max(vals), "sum": total,
               "skipped": skipped})


if __name__ == "__main__":
    sys.exit(main())
