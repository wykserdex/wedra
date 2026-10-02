#!/usr/bin/env python3
"""ghauri — поиск SQL-инъекций на явно заданной цели (обёртка над CLI ghauri).

Вход (stdin JSON): url (http/https), timeout (опц., с/запрос, 30),
wall_timeout (опц., общий лимит рана, 600).

Вызов: <GHAURI_BIN|ghauri> -u <url> --batch --level 1 --timeout N
       (cwd = временная папка; HOME/USERPROFILE дочернего процесса тоже
       временные — ghauri пишет сессию в ~/.ghauri/<netloc>/, наружу от рана
       ничего не остаётся).

Машинного формата у ghauri нет: отчёт — текстовый лог ~/.ghauri/<host>/log,
его пишет FileHandler с форматом "%(message)s" (без времени и уровня), но
внутри сообщения остаются ANSI-последовательности от colorize(), поэтому перед
разбором коды вырезаются. Признак инъекции — строка уровня NOTICE
"GET parameter 'id' appears to be '<TITLE>' injectable" (ghauri/core/tests.py,
logger.notice); итог отсутствия уязвимости — "all tested parameters do not
appear to be injectable.".

Аудит неинвазивный и только по цели, которую явно задал пользователь: плагин НЕ
передаёт флаги перечисления и эксплуатации (--dbs/--tables/--columns/--dump/
--sql-shell и т. п.), поэтому ghauri остаётся детектором уровня 1 по
GET/POST-параметрам — никакой записи файлов на цель и никаких команд ОС.

Выход (stdout JSON): {url, vulnerable, parameter,
injections[{parameter, place, technique}]}. Не нашлось инъекций — это ok с
vulnerable=false, а не ошибка. Доменные ошибки: empty_url, bad_url (не
http(s)-схема либо в URL нет проверяемых параметров), ghauri_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый
JSON входа, нечисловые timeout/wall_timeout.
"""
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

DEFAULT_TIMEOUT = 30
DEFAULT_WALL = 600

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
NO_PARAMS_RE = re.compile(r"no parameter\(s\) found for testing", re.I)
INJ_RE = re.compile(
    r"(?:(?P<place>GET|POST|URI|COOKIE|HEADER)\s+)?"
    r"(?:\(custom\)\s+)?parameter\s+'(?P<param>[^']+)'\s+"
    r"appears to be\s+'(?P<technique>[^']+)'\s+injectable")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def resolve(bin_env):
    if "/" in bin_env or "\\" in bin_env:
        return os.path.abspath(bin_env)
    return shutil.which(bin_env) or os.path.abspath(bin_env)


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")
    if not (url.startswith("http://") or url.startswith("https://")):
        return fail("bad_url", "url должен начинаться с http:// или https://")

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = [resolve(os.environ.get("GHAURI_BIN", "ghauri").strip() or "ghauri"),
           "-u", url, "--batch", "--level", "1",
           "--timeout", str(int(timeout))]
    if cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        env = dict(os.environ)
        env["HOME"] = td
        env["USERPROFILE"] = td
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, env=env, timeout=wall)
        except FileNotFoundError:
            return fail("ghauri_not_installed",
                        "ghauri не найден в PATH: pip install ghauri "
                        "(или укажите GHAURI_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"ghauri не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"ghauri упал: {last}")

        logs = sorted(glob.glob(os.path.join(td, ".ghauri", "*", "log")))
        if not logs:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else "лог не создан"
            return fail("no_report", f"ghauri не дал лог: {last}")
        log_path = logs[-1]
        try:
            with open(log_path, encoding="utf-8", errors="replace") as f:
                text = f.read()
        except Exception as e:
            return fail("bad_report", f"не прочитан лог ghauri: {e}",
                        exit_code=2)
        if not text.strip():
            return fail("no_report", "лог ghauri пуст")

    if NO_PARAMS_RE.search(ANSI_RE.sub("", text)):
        return fail("bad_url",
                    "в URL нет проверяемых параметров — добавьте query "
                    "(например ?id=1) или POST-данные")

    injections = []
    seen = set()
    for line in text.splitlines():
        for m in INJ_RE.finditer(ANSI_RE.sub("", line)):
            item = {
                "parameter": m.group("param").strip(),
                "place": (m.group("place") or "").strip(),
                "technique": m.group("technique").strip(),
            }
            key = (item["parameter"], item["place"], item["technique"])
            if key in seen:
                continue
            seen.add(key)
            injections.append(item)

    return ok({"url": url, "vulnerable": bool(injections),
               "parameter": injections[0]["parameter"] if injections else "",
               "injections": injections})


if __name__ == "__main__":
    sys.exit(main())