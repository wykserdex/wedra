#!/usr/bin/env python3
"""text_analyzer — метрики текста: строки, слова, уникальные слова, длиннейшее слово.

Community-плагин №1 — написан внешним автором (тестер №1, M5) строго по
PROTOCOL.md, без взгляда на код ядра. Воспроизведён в репо по его фидбеку.
Без зависимостей, без сети, без файлов.
"""
import json
import re
import sys


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON: {e}", exit_code=2)

    text = str(data.get("text") or "").strip()
    if not text:
        return fail("empty_text", "поле text пустое")

    lines = text.count("\n") + 1
    words = re.findall(r"\w+", text, flags=re.UNICODE)
    unique = {w.lower() for w in words}
    longest = max(words, key=len) if words else ""

    print(json.dumps({"status": "ok", "output": {
        "lines": lines,
        "words": len(words),
        "unique_words": len(unique),
        "longest_word": longest,
    }}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
