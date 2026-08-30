#!/usr/bin/env python3
"""text_similarity — расстояние Левенштейна + нормализованное сходство двух строк.

Флаг near-duplicate (similar) считается по порогу threshold (0..1, дефолт 0.8) —
удобный шаг перед human_gate при поиске дубликатов. Без сети и секретов.
"""
import json
import sys


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


def levenshtein(a, b):
    """Классический DP с двумя рядами — O(len(a) * len(b)), память O(len(b))."""
    if a == b:
        return 0
    prev = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        cur = [i]
        for j, cb in enumerate(b, 1):
            cur.append(min(
                cur[j - 1] + 1,        # вставка
                prev[j] + 1,           # удаление
                prev[j - 1] + (ca != cb),  # замена
            ))
        prev = cur
    return prev[-1]


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:  # noqa: BLE001
        return platform_bad_input("stdin не является JSON: %s" % e)

    # Guard по типам: у нас две строки на входе — и обе должны быть строками
    if (not isinstance(data, dict)
            or not isinstance(data.get("a"), str)
            or not isinstance(data.get("b"), str)):
        return platform_bad_input("ожидался объект {a: string, b: string}")

    a, b = data["a"], data["b"]

    if not a.strip() and not b.strip():
        return fail("empty_input", "обе строки пустые")

    threshold = data.get("threshold", 0.8)
    if isinstance(threshold, bool) or not isinstance(threshold, (int, float)):
        return fail("bad_threshold", "threshold должен быть числом")
    if threshold < 0 or threshold > 1:
        return fail("bad_threshold", "threshold должен быть в диапазоне [0, 1]")

    distance = levenshtein(a, b)
    max_len = max(len(a), len(b))
    similarity = round(1.0 - distance / max_len, 4) if max_len > 0 else 0.0

    return ok({
        "distance": distance,
        "max_len": max_len,
        "similarity": similarity,
        "similar": similarity >= threshold,
        "threshold": threshold,
        "chars": {"a": len(a), "b": len(b)},
    })


if __name__ == "__main__":
    sys.exit(main())
