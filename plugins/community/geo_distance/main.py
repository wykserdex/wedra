#!/usr/bin/env python3
"""geo_distance — гаверсинус между двумя точками (км/метры/мили)."""
import json
import math
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


EARTH_KM = 6371.0088


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    try:
        lat1 = float(data["lat1"])
        lon1 = float(data["lon1"])
        lat2 = float(data["lat2"])
        lon2 = float(data["lon2"])
    except (KeyError, TypeError, ValueError):
        return fail("bad_coords", "нужны числа lat1/lon1/lat2/lon2")
    if not (-90 <= lat1 <= 90 and -90 <= lat2 <= 90
            and -180 <= lon1 <= 180 and -180 <= lon2 <= 180):
        return fail("bad_coords", "широта ±90, долгота ±180")

    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp = math.radians(lat2 - lat1)
    dl = math.radians(lon2 - lon1)
    a = (math.sin(dp / 2) ** 2
         + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2)
    km = round(2 * EARTH_KM * math.asin(math.sqrt(a)), 3)
    return ok({"km": km, "meters": round(km * 1000, 1),
               "miles": round(km * 0.621371, 3)})


if __name__ == "__main__":
    sys.exit(main())
