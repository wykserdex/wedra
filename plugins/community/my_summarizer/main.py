#!/usr/bin/env python3
"""my_summarizer — плагин оркестратора (сгенерировано: tool plugin create).

Протокол (PROTOCOL.md v0.1):
  stdin  ← JSON со входом: ядро собирает его из полей input манифеста
  stdout → ровно один JSON-конверт:
           ok:    {"status": "ok", "output": {...}}               exit 0
           error: {"status": "error", "error": {...}}             exit 1 (доменная ошибка)
           исключение / мусор на stdout → платформенная ошибка →   exit 2
  stderr → свободные логи, ядро сложит их в журнал рана

Граница ответственности: плагин СООБЩАЕТ об ошибке; судьбу цепочки
(stop/skip/retry) решает конфиг пайплайна, а не плагин.
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


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        # виновата вызывающая сторона — платформенная ошибка, стопит ран
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    text = str(data.get("text") or "").strip()
    if not text:
        # доменная ошибка: шаг отработал штатно, но результат отрицательный
        return fail("empty_input", "поле text пустое")

    # ── ваша логика здесь ──────────────────────────────────────
    words = len(text.split())
    chars = len(text)
    return ok({"words": words, "chars": chars})


if __name__ == "__main__":
    sys.exit(main())
