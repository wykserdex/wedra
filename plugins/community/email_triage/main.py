#!/usr/bin/env python3
"""email_triage — риск-скоринг email с optional syntax/disposable результатами.

Если передан syntax_ok=false, адрес всё равно возвращается в triage как bad,
вместо того чтобы прерывать обработку локальной проверкой формата.
"""
import json
import sys


def fail(code, message, exit_code=1):
    print(json.dumps({"status": "error", "error": {
        "code": code, "message": message, "retryable": False}},
        ensure_ascii=False))
    return exit_code


def fail_platform(code, message):
    return fail(code, message, exit_code=2)


def _looks_like_email(email):
    if "@" not in email:
        return False
    local, domain = email.rsplit("@", 1)
    return bool(local and domain and "." in domain)


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

    syntax_ok = data.get("syntax_ok")
    disposable = data.get("disposable")
    if syntax_ok is not None and not isinstance(syntax_ok, bool):
        return fail_platform("bad_input", f"syntax_ok должен быть boolean, пришло {type(syntax_ok).__name__}")
    if disposable is not None and not isinstance(disposable, bool):
        return fail_platform("bad_input", f"disposable должен быть boolean, пришло {type(disposable).__name__}")

    # Когда syntax checker уже вернул False, это входной сигнал для triage,
    # а не повод завершать plugin с bad_syntax. Без такого сигнала сохраняем
    # локальный guard для очевидно некорректных адресов.
    if syntax_ok is not False and not _looks_like_email(email):
        return fail("bad_syntax", f"не похоже на email: {email}")

    reasons = []
    risk = 10
    if syntax_ok is False:
        risk = max(risk, 90)
        reasons.append("bad_syntax")
    if disposable is True:
        risk = max(risk, 80)
        reasons.append("disposable")
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

    print(json.dumps({"status": "ok", "output": {
        "risk": risk, "verdict": verdict, "reasons": reasons}},
        ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
