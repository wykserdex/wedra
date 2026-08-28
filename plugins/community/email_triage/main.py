#!/usr/bin/env python3
"""email_triage — фан-ин триаж email с optional портами.

Входы:
  email (required, format email)
  syntax_ok (optional bool) — результат syntax_mx_checker
  disposable (optional bool) — результат disposable_checker

Выходы:
  risk (0..100), verdict (good/bad/suspicious), reasons (array)

Логика для тестов:
  - пустой email → empty_input (exit 1)
  - syntax_ok == False → risk 90, verdict bad, reasons ["bad_syntax"]
  - disposable == True → risk 80, verdict bad, reasons ["disposable"]
  - оба плохих → risk 99, verdict bad, reasons ["bad_syntax","disposable"]
  - иначе risk 10, verdict good
"""
import json
import sys
import re


def fail(code, message, exit_code=1):
    print(json.dumps({"status": "error", "error": {"code": code, "message": message, "retryable": False}}, ensure_ascii=False))
    return exit_code


def fail_platform(code, message):
    print(json.dumps({"status": "error", "error": {"code": code, "message": message, "retryable": False}}, ensure_ascii=False))
    return 2


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail_platform("bad_input", f"невалидный JSON: {e}")

    email = data.get("email")
    if email is None:
        return fail("empty_input", "поле email отсутствует")
    if not isinstance(email, str):
        return fail_platform("bad_input", f"email должен быть строкой, пришло {type(email).__name__}")
    email = email.strip()
    if not email:
        return fail("empty_input", "поле email пустое")

    # простая проверка формата (валидатор уже проверяет, но runtime guard)
    if "@" not in email or "." not in email.split("@")[-1]:
        return fail("bad_syntax", f"не похоже на email: {email}")

    syntax_ok = data.get("syntax_ok")
    disposable = data.get("disposable")

    # optional порты могут отсутствовать — это нормально (проверка бага #10)
    if syntax_ok is not None and not isinstance(syntax_ok, bool):
        return fail_platform("bad_input", f"syntax_ok должен быть boolean, пришло {type(syntax_ok).__name__}")
    if disposable is not None and not isinstance(disposable, bool):
        return fail_platform("bad_input", f"disposable должен быть boolean, пришло {type(disposable).__name__}")

    reasons = []
    risk = 10

    if syntax_ok is False:
        risk = max(risk, 90)
        reasons.append("bad_syntax")
    if disposable is True:
        risk = max(risk, 80)
        reasons.append("disposable")
        # если оба плохих — риск 99 как в примере фидбека
        if syntax_ok is False:
            risk = 99

    if risk >= 80:
        verdict = "bad"
    elif risk >= 40:
        verdict = "suspicious"
    else:
        verdict = "good"

    if not reasons and verdict == "good":
        reasons = ["clean"]

    print(json.dumps({"status": "ok", "output": {"risk": risk, "verdict": verdict, "reasons": reasons}}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
