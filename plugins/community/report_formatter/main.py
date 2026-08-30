#!/usr/bin/env python3
"""report_formatter — превращает per-item результаты триажа в текстовый
отчёт (string), чтобы его можно было скормить LLM-плагину или показать
аналитику на human_gate одним полем.

Мостик между batch_email_triage (array/object на выходе) и llm_* (string
на входе) — типы не приводятся автоматически, нужен явный шаг форматирования.

Протокол (PROTOCOL.md):
  stdin  ← JSON {"results": [...], "by_verdict": {...}}
  stdout → status: ok/error
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
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    results = data.get("results")
    by_verdict = data.get("by_verdict")
    if not isinstance(results, list) or not isinstance(by_verdict, dict):
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input",
            "message": "ожидались results: array и by_verdict: object",
            "retryable": False}}, ensure_ascii=False))
        return 2

    if not results:
        return fail("empty_input", "results пуст — нечего форматировать")

    lines = [f"Триаж {len(results)} email:"]
    lines.append(", ".join(f"{k}={v}" for k, v in sorted(by_verdict.items())))
    lines.append("")
    for r in results:
        flag = "⚠" if r.get("verdict") != "ok" else "·"
        lines.append(f"{flag} {r.get('email')}: {r.get('verdict')}")

    return ok({"summary": "\n".join(lines)})


if __name__ == "__main__":
    sys.exit(main())
