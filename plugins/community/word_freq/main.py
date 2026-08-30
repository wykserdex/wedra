#!/usr/bin/env python3
"""word_freq — топ-N самых частых слов в тексте (без стоп-слов длиной 1-2 символа)."""
import json
import re
import sys
from collections import Counter


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return 1


WORD_RE = re.compile(r"[a-zA-Zа-яА-ЯёЁ]+")


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    text = str(data.get("text") or "").strip()
    if not text:
        return fail("empty_input", "поле text пустое")

    top_n = data.get("top_n")
    if top_n is None:
        top_n = 5
    try:
        top_n = int(top_n)
    except (TypeError, ValueError):
        return fail("bad_top_n", f"top_n должен быть числом, получено {top_n!r}")
    if top_n <= 0:
        return fail("bad_top_n", "top_n должен быть положительным")

    words = [w.lower() for w in WORD_RE.findall(text) if len(w) > 2]
    if not words:
        return fail("no_words", "в тексте не найдено слов длиннее 2 символов")

    counts = Counter(words)
    top = counts.most_common(top_n)

    return ok({
        "total_words": len(words),
        "unique_words": len(counts),
        "top": [{"word": w, "count": c} for w, c in top],
    })


if __name__ == "__main__":
    sys.exit(main())
