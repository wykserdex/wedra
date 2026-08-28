#!/usr/bin/env python3
"""llm_openai — OpenAI-совместимый chat/completions (только stdlib).

Покрывает OpenAI, xAI/Grok, DeepSeek, OpenRouter, vLLM/Ollama —
провайдер выбирается env'ами:

  LLM_OAI_BASE_URL  — default https://api.openai.com/v1
                        (для Grok: https://api.x.ai/v1)
  LLM_OAI_API_KEY   — ключ
  LLM_OAI_MODEL     — default gpt-4o-mini (для Grok: grok-3-mini)
  LLM_MOCK=1        — детерминированный ответ без сети
  LLM_TIMEOUT       — таймаут HTTP, default 30с
"""
import json
import os
import sys
import urllib.error
import urllib.request

BASE_URL = os.environ.get("LLM_OAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
MODEL = os.environ.get("LLM_OAI_MODEL", "gpt-4o-mini")
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
        return ok(f"[mock:{MODEL}] доработанный текст: {prompt[:80]}")

    key = os.environ.get("LLM_OAI_API_KEY", "")
    if not key:
        return fail("no_api_key", "нет LLM_OAI_API_KEY (ожидается в env)")

    messages = ([{"role": "system", "content": system}] if system else []) + \
               [{"role": "user", "content": prompt}]
    payload = {"model": MODEL, "messages": messages}

    req = urllib.request.Request(
        f"{BASE_URL}/chat/completions",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json",
                 "Authorization": f"Bearer {key}"})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = json.load(resp)
    except urllib.error.HTTPError as e:
        retryable = e.code == 429 or e.code >= 500
        return fail(f"http_{e.code}", f"provider HTTP {e.code}", retryable=retryable)
    except Exception as e:
        return fail("network", str(e), retryable=True)

    try:
        text = body["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError) as e:
        print(f"неожиданный ответ: {body!r}"[:500], file=sys.stderr)
        return fail("bad_response", f"не распарсился ответ: {e}", retryable=True)
    return ok(text)


if __name__ == "__main__":
    sys.exit(main())
