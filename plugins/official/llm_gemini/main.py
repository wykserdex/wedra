#!/usr/bin/env python3
"""llm_gemini — адаптер к Gemini generateContent API (только stdlib).

Контракт: PROTOCOL.md v0.1.

env:
  GEMINI_API_KEY   — ключ (обязателен без mock-режима)
  GEMINI_MODEL     — модель, default: gemini-2.0-flash
  GEMINI_BASE_URL  — переопределение endpoint'а (тесты/прокси)
  LLM_MOCK=1       — детерминированный ответ без сети (демо/тесты)
  LLM_TIMEOUT      — таймаут HTTP в секундах, default 30
"""
import json
import os
import sys
import urllib.error
import urllib.request

BASE_URL = os.environ.get(
    "GEMINI_BASE_URL", "https://generativelanguage.googleapis.com").rstrip("/")
MODEL = os.environ.get("GEMINI_MODEL", "gemini-2.0-flash")
TIMEOUT = float(os.environ.get("LLM_TIMEOUT", "30"))


def ok(text):
    print(json.dumps({"status": "ok",
                      "output": {"text": text, "model": MODEL}}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)

    prompt = str(data.get("prompt") or "").strip()
    system = str(data.get("system") or "").strip()
    if not prompt:
        return fail("empty_prompt", "пустой prompt")

    if os.environ.get("LLM_MOCK") == "1":
        return ok(f"[mock:{MODEL}] черновик по теме: {prompt[:80]}")

    key = os.environ.get("GEMINI_API_KEY", "")
    if not key:
        return fail("no_api_key", "нет GEMINI_API_KEY (ожидается в env)")

    payload = {"contents": [{"parts": [{"text": prompt}]}]}
    if system:
        payload["system_instruction"] = {"parts": [{"text": system}]}

    url = f"{BASE_URL}/v1beta/models/{MODEL}:generateContent?key={key}"
    req = urllib.request.Request(
        url, data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = json.load(resp)
    except urllib.error.HTTPError as e:
        retryable = e.code == 429 or e.code >= 500
        return fail(f"http_{e.code}", f"Gemini HTTP {e.code}", retryable=retryable)
    except Exception as e:  # DNS, TCP, TLS, таймаут — сетевые сбои ретраим
        return fail("network", str(e), retryable=True)

    try:
        text = body["candidates"][0]["content"]["parts"][0]["text"]
    except (KeyError, IndexError, TypeError) as e:
        print(f"неожиданный ответ Gemini: {body!r}"[:500], file=sys.stderr)
        return fail("bad_response", f"не распарсился ответ Gemini: {e}",
                    retryable=True)
    return ok(text)


if __name__ == "__main__":
    sys.exit(main())
