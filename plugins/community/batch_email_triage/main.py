#!/usr/bin/env python3
"""batch_email_triage — синтаксис+disposable-триаж СПИСКА email одним вызовом.

Обходной путь для реального ограничения core foreach (найдено при тестировании
v8): foreach оборачивает ВЕСЬ список шагов на каждый элемент — нет способа
получить ОДИН сводный шаг после батча (см. FEEDBACK_v8.md). Этот плагин решает
тот же класс задач ("батч-проверка → один отчёт для аналитика") без foreach:
сам разворачивает цикл внутри одного шага, отдаёт массив per-item результатов
+ агрегированную статистику одним выходом — который дальше можно скормить
LLM-плагину или human_gate ОДИН раз на весь батч.

Протокол (PROTOCOL.md):
  stdin  ← JSON {"items": ["a@b.com", ...]}
  stdout → status: ok/error
"""
import json
import sys

DISPOSABLE = {
    "mailinator.com", "tempmail.com", "temp-mail.org", "10minutemail.com",
    "guerrillamail.com", "yopmail.com", "trashmail.com", "trashmail.net",
    "sharklasers.com", "guerrillamailblock.com", "grr.la", "dispostable.com",
    "maildrop.cc", "fakeinbox.com", "getnada.com", "mohmal.com",
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


def check_one(raw):
    email = str(raw or "").strip()
    if "@" not in email or " " in email or email.startswith("@") or email.endswith("@"):
        return {"email": email, "valid_syntax": False, "disposable": None,
                "verdict": "bad_syntax"}
    domain = email.split("@")[-1].lower()
    disposable = domain in DISPOSABLE
    verdict = "disposable" if disposable else "ok"
    return {"email": email, "valid_syntax": True, "disposable": disposable,
            "verdict": verdict}


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    items = data.get("items")
    if not isinstance(items, list):
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input",
            "message": "items должен быть массивом (type: array в манифесте)",
            "retryable": False}}, ensure_ascii=False))
        return 2

    if len(items) == 0:
        return fail("empty_batch", "пустой список email — нечего проверять")

    results = [check_one(x) for x in items]
    by_verdict = {}
    for r in results:
        by_verdict[r["verdict"]] = by_verdict.get(r["verdict"], 0) + 1

    return ok({
        "total": len(results),
        "results": results,
        "by_verdict": by_verdict,
    })


if __name__ == "__main__":
    sys.exit(main())
