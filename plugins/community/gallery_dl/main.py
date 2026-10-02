#!/usr/bin/env python3
"""gallery_dl — метаданные изображений/галерей по URL (обёртка над gallery-dl).

Вход (stdin JSON): url (обязателен), wall_timeout (опц., общий лимит, 300).

Вызов: <GALLERY_DL_BIN|gallery-dl|python3 -m gallery_dl> -j --no-download <url>
       (cwd = временная папка).

Режим «только метаданные». Флаги сверены с исходниками mikf/gallery-dl
(gallery_dl/option.py, gallery_dl/__init__.py, gallery_dl/job.py, ветка master):
  -j/--dump-json — печатает JSON-информацию в stdout и переключает job на
        DataJob («Collect extractor results and dump them»), который файлы не
        качает вообще (в DataJob нет загрузчика);
  --no-download   — «Do not download any files»: страховка от записи файлов
        даже на конфигу, где вывод переключается в DownloadJob.

Формат отчёта проверен по коду DataJob (gallery_dl/job.py): в stdout идёт JSON-
массив сообщений-кортежей, где первый элемент — идентификатор из
gallery_dl/extractor/message.py: Message.Url = 3 → [3, <url>, <метаданные>],
Message.Directory = 2 → [2, <метаданные галереи>], Message.Queue = 6 →
[6, <внешний url>, <метаданные>]. Позициями галереи считаются только
Message.Url; заголовок берётся из Message.Directory, а если его нет — из
метаданных первой позиции (title, иначе gallery_title, иначе category).

Выход (stdout JSON): {url, items[{index, url, category, extension}], count,
title}. Пустая галерея — ok с пустым items, а не ошибка. Доменные ошибки:
empty_url, bad_url (аргумент начинается с - либо содержит пробелы),
gallery_dl_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечисловой wall_timeout, нечитаемый
отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

MESSAGE_URL = 3
MESSAGE_DIRECTORY = 2

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)
RETRY_RE = re.compile(
    r"\b(429|500|502|503|504)\b|Too Many Requests|timed out|connection "
    r"reset|connection aborted|Temporary failure|temporarily unavailable",
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
    bin_env = os.environ.get("GALLERY_DL_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("gallery-dl")
    if found:
        return [found]
    return [sys.executable, "-m", "gallery_dl"]


def as_text(value):
    if value is None or isinstance(value, (list, dict, bool)):
        return ""
    if isinstance(value, str):
        return value
    return str(value)


def as_index(value, fallback):
    if isinstance(value, bool) or value is None:
        return fallback
    try:
        return int(value)
    except (TypeError, ValueError):
        return fallback


def json_values(text):
    """JSON-значения верхнего уровня из stdout: сперва пробуем разобрать весь
    вывод (штатный случай — один JSON-массив сообщений), иначе вырезаем значения
    по скобкам (output.jsonl печатает по одному объекту на строку)."""
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


def is_message(value):
    """Сообщение DataJob: [Message.Url, url, meta] или [Message.Directory,
    meta], либо одиночный словарь метаданных. Первый элемент списка — всегда
    целое число-идентификатор из gallery_dl/extractor/message.py."""
    if isinstance(value, dict):
        return True
    if not isinstance(value, list) or len(value) not in (2, 3):
        return False
    head = value[0]
    return isinstance(head, int) and not isinstance(head, bool)


def flatten_report(values):
    """Значения верхнего уровня → плоский список сообщений: массив-отчёт
    разворачивается, а пообъектный вывод output.jsonl добавляется как есть."""
    entries = []
    for value in values:
        if isinstance(value, list) and value and all(is_message(v)
                                                    for v in value):
            entries.extend(value)
        elif is_message(value):
            entries.append(value)
    return entries


def split_message(entry):
    """Сообщение DataJob → (тип, url, метаданные)."""
    if isinstance(entry, list) and len(entry) >= 3 and isinstance(entry[2], dict):
        return entry[0], entry[1], entry[2]
    if isinstance(entry, list) and len(entry) == 2 and isinstance(entry[1], dict):
        return entry[0], "", entry[1]
    if isinstance(entry, dict):
        return MESSAGE_URL, "", entry
    return None, "", {}


def first_title(meta):
    for key in ("title", "gallery_title", "category"):
        value = as_text(meta.get(key))
        if value:
            return value
    return ""


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
    cmd += ["-j", "--no-download", url]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("gallery_dl_not_installed",
                        "gallery-dl не найден: pip install gallery-dl "
                        "(или укажите GALLERY_DL_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"gallery-dl не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        blob = stdout + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("gallery_dl_not_installed",
                        "gallery-dl не установлен: pip install gallery-dl "
                        "(или укажите GALLERY_DL_BIN)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"gallery-dl упал (exit {proc.returncode}): {last}",
                    retryable=bool(RETRY_RE.search(blob)))

    if not stdout.strip():
        return fail("no_report", "gallery-dl не дал JSON в stdout")

    values = json_values(stdout)
    if not values:
        return fail("bad_report", "не разобран JSON от gallery-dl",
                    exit_code=2)
    entries = flatten_report(values)
    if not entries:
        return fail("bad_report",
                    "неожиданный формат JSON от gallery-dl: ни одного "
                    "сообщения Message", exit_code=2)

    items = []
    title = ""
    for entry in entries:
        kind, item_url, meta = split_message(entry)
        if kind is None:
            continue
        if kind == MESSAGE_DIRECTORY:
            title = title or first_title(meta)
            continue
        if kind != MESSAGE_URL:
            continue
        position = len(items) + 1
        items.append({
            "index": as_index(meta.get("num", meta.get("index")), position),
            "url": as_text(item_url) or as_text(meta.get("url")),
            "category": as_text(meta.get("category")),
            "extension": as_text(meta.get("extension")),
        })
        title = title or first_title(meta)

    return ok({"url": url, "items": items, "count": len(items),
               "title": title})


if __name__ == "__main__":
    sys.exit(main())
