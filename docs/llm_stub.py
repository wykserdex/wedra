#!/usr/bin/env python3
"""Локальный заглушечный LLM — чтобы пройти LLM-цепочку без сети и без ключей.

Зачем он: переменная LLM_MOCK, которую раньше рекомендовал quickstart, ядро
больше не пропускает в плагин (см. CHANGELOG, раздел про запрет тестовых
переключателей в permissions.secrets). Зато GEMINI_BASE_URL и LLM_OAI_BASE_URL
объявлены в манифестах плагинов как secrets — значит, до них значение доходит.
Этот скрипт поднимает их на 127.0.0.1 и отвечает в форматах обоих провайдеров.

Запуск:  python docs/llm_stub.py
         (оставьте окно открытым, в нём будет видно каждый запрос)

Что делает ответ: возвращает ВХОД плагина обратно с префиксом. Поэтому во втором
шаге видно правку человека — цепочка проверена по-настоящему, а не заглушкой.
"""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 8766
HOST = "127.0.0.1"

# На русской Windows консоль часто cp866/cp1251, и print() кириллицы там
# роняет процесс UnicodeEncodeError — то есть заглушка умирает ровно у того
# читателя, ради которого написана. Поэтому stdout переводим в utf-8, а на
# консоли без utf-8 разрешаем замену вместо исключения.
for _stream in (sys.stdout, sys.stderr):
    try:
        _stream.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass


def _gemini_prompt(body):
    try:
        c = body["contents"][0]["parts"][0]["text"]
    except (KeyError, IndexError, TypeError):
        return ""
    return c


def _openai_prompt(body):
    for m in body.get("messages") or []:
        if m.get("role") == "user":
            return m.get("content") or ""
    return ""


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):  # noqa: N802 — имя задано BaseHTTPRequestHandler
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        try:
            body = json.loads(raw or b"{}")
        except ValueError:
            body = {}

        if "/chat/completions" in self.path:
            text = "эхо-редактор: " + _openai_prompt(body)
            reply = {"choices": [{"message": {"content": text}}]}
            kind = "openai"
        else:
            text = "эхо-черновик: " + _gemini_prompt(body)
            reply = {"candidates": [{"content": {"parts": [{"text": text}]}}]}
            kind = "gemini"

        print(f"  <- {kind} {self.path.split('?')[0]}  вернул: {text!r}", flush=True)

        out = json.dumps(reply).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    srv = HTTPServer((HOST, PORT), Handler)
    print(f"Заглушка LLM слушает http://{HOST}:{PORT} — Ctrl+C, когда закончишь.", flush=True)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        print("\nОстановлено.")
        sys.exit(0)