#!/usr/bin/env python3
"""you_get — метаданные медиа с видеосервисов по URL (обёртка над CLI you-get).

Вход (stdin JSON): url (обязателен), wall_timeout (опц., общий лимит, 300).

Вызов: <YOU_GET_BIN|you-get|python3 -m you_get> --json <url>  (cwd = временная
папка).

Режим «только метаданные». Сверено с исходниками soimort/you-get (ветка
develop): флага `--skip-download` в CLI нет вообще (в отличие от слухов в
блогах), документированные режимы без скачивания — `-i/--info` (текст) и
`--json` (JSON). Берём `--json`: в you_get/common.py он ставит
json_output=True и dry_run=True, а VideoExtractor.download() (you_get/
extractor.py) первым делом уходит в json_output.output(self) и до веток
загрузки не доходит; common.download_urls() при json_output тоже печатает JSON
и возвращается. Файлы не пишутся.

Формат отчёта (you_get/json_output.py, output()): JSON-объект с ключами url,
title, site (имя экстрактора), streams (словарь потоков, в который слиты
dash_streams), опционально audiolang и extra. Печатается с indent=4 и
ensure_ascii=False. Ключей uploader и duration в схеме нет — они остаются
пустой строкой и нулём, если экстрактор их не отдал; streams считается по
длине словаря streams. Если в stdout несколько JSON-объектов, берётся последний.

Выход (stdout JSON): {url, title, uploader, duration, extractor, streams}.
Пустой stdout — no_report, нечитаемый/необъектный JSON — bad_report (exit 2).
Доменные ошибки: empty_url, bad_url (аргумент начинается с - либо содержит
пробелы), you_get_not_installed, timeout (retryable), no_report, tool_failed
(со 429/5xx/таймаутом — retryable). Платформенные (exit 2): битый JSON входа,
нечисловой wall_timeout, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

NOT_MODULE_RE = re.compile(r"No module named|ModuleNotFoundError",
                           re.IGNORECASE)
RETRY_RE = re.compile(
    r"HTTP Error (429|5\d\d)|Too Many Requests|timed out|connection reset|"
    r"connection aborted|Temporary failure|temporarily unavailable",
    re.IGNORECASE,
)


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("YOU_GET_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("you-get")
    if found:
        return [found]
    return [sys.executable, "-m", "you_get"]


def as_text(value):
    if value is None or isinstance(value, (list, dict, bool)):
        return ""
    if isinstance(value, str):
        return value
    return str(value)


def as_number(value):
    if value is None or isinstance(value, (list, dict, bool)):
        return 0
    try:
        number = float(value)
    except (TypeError, ValueError):
        return 0
    if number != number or number in (float("inf"), float("-inf")):
        return 0
    return int(number) if number.is_integer() else number


def json_values(text):
    """JSON-значения верхнего уровня из stdout: сперва пробуем разобрать весь
    вывод (штатный случай — один JSON-объект), иначе вырезаем объекты по
    скобкам, пропуская мусор между ними."""
    blob = (text or "").strip()
    if not blob:
        return []
    try:
        return [json.loads(blob)]
    except ValueError:
        pass
    found = []
    depth = 0
    start = -1
    in_string = False
    escaped = False
    for pos, ch in enumerate(blob):
        if in_string:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == '"':
                in_string = False
            continue
        if ch == '"':
            in_string = True
        elif ch in "{[":
            if depth == 0:
                start = pos
            depth += 1
        elif ch in "}]":
            if depth > 0:
                depth -= 1
                if depth == 0 and start >= 0:
                    try:
                        found.append(json.loads(blob[start:pos + 1]))
                    except ValueError:
                        pass
                    start = -1
    return found


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8", errors="replace")
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")
    if url.startswith("-") or any(c.isspace() for c in url):
        return fail("bad_url",
                    "url должен быть одним аргументом: без пробелов и не "
                    "начинаться с '-'")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += ["--json", url]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("you_get_not_installed",
                        "you-get не найден: pip install you-get "
                        "(или укажите YOU_GET_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"you-get не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        blob = stdout + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("you_get_not_installed",
                        "you-get не установлен: pip install you-get "
                        "(или укажите YOU_GET_BIN)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"you-get упал (exit {proc.returncode}): {last}",
                    retryable=bool(RETRY_RE.search(blob)))

    if not stdout.strip():
        return fail("no_report", "you-get не дал JSON в stdout")

    report = None
    for value in reversed(json_values(stdout)):
        if isinstance(value, dict):
            report = value
            break
    if report is None:
        return fail("bad_report", "не разобран JSON от you-get", exit_code=2)

    streams = report.get("streams")
    return ok({
        "url": as_text(report.get("url")) or url,
        "title": as_text(report.get("title")),
        "uploader": as_text(report.get("uploader") or report.get("author")),
        "duration": as_number(report.get("duration")),
        "extractor": as_text(report.get("site") or report.get("extractor")),
        "streams": len(streams) if isinstance(streams, dict) else 0,
    })


if __name__ == "__main__":
    sys.exit(main())
