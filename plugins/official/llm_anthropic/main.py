#!/usr/bin/env python3
"""llm_anthropic — адаптер к Anthropic Messages API (только stdlib).

env:
  ANTHROPIC_API_KEY   — ключ
  ANTHROPIC_MODEL     — default claude-3-5-haiku-latest
  ANTHROPIC_BASE_URL  — default https://api.anthropic.com (тесты/прокси)
  LLM_MOCK=1          — детерминированный ответ без сети
  LLM_TIMEOUT         — таймаут HTTP, default 30с
"""
import json
import os
import sys
import urllib.error
import urllib.request

BASE_URL = os.environ.get("ANTHROPIC_BASE_URL", "https://api.anthropic.com").rstrip("/")
MODEL = os.environ.get("ANTHROPIC_MODEL", "claude-3-5-haiku-latest")
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
        return ok(f"[mock:{MODEL}] ответ по теме: {prompt[:80]}")

    key = os.environ.get("ANTHROPIC_API_KEY", "")
    if not key:
        return fail("no_api_key", "нет ANTHROPIC_API_KEY (ожидается в env)")

    payload = {
        "model": MODEL,
        "max_tokens": 1024,
        "messages": [{"role": "user", "content": prompt}],
    }
    if system:
        payload["system"] = system

    req = urllib.request.Request(
        f"{BASE_URL}/v1/messages",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json",
                 "x-api-key": key,
                 "anthropic-version": "2023-06-01"})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = json.load(resp)
    except urllib.error.HTTPError as e:
        retryable = e.code == 429 or e.code >= 500
        return fail(f"http_{e.code}", f"Anthropic HTTP {e.code}", retryable=retryable)
    except Exception as e:
        return fail("network", str(e), retryable=True)

    try:
        text = body["content"][0]["text"]
    except (KeyError, IndexError, TypeError) as e:
        print(f"неожиданный ответ Anthropic: {body!r}"[:500], file=sys.stderr)
        return fail("bad_response", f"не распарсился ответ Anthropic: {e}",
                    retryable=True)
    return ok(text)


if __name__ == "__main__":
    sys.exit(main())
