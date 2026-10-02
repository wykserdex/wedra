#!/usr/bin/env python3
"""yt_dlp — метаданные видео по URL (обёртка над CLI yt-dlp), без скачивания медиа.

Вход (stdin JSON): url (обязателен), playlist (опц., boolean, по умолчанию
false), wall_timeout (опц., общий лимит, 300).

Вызов: <YT_DLP_BIN|yt-dlp|python3 -m yt_dlp> -J --skip-download --ignore-config
       --no-warnings --no-progress --no-playlist|--yes-playlist <url>
       (cwd = временная папка).

Режим «только метаданные». Флаги сверены с исходниками yt-dlp
(yt_dlp/options.py, ветка master):
  -J/--dump-single-json — один большой JSON-объект в stdout; сам по себе
        включает simulate, то есть медиа не скачивается;
  --skip-download      — явный запрет скачивания (дублирует simulate);
  --no-playlist / --yes-playlist — по умолчанию берётся только ролик, даже если
        в URL есть параметр list=..., при playlist=true разбирается весь
        плейлист целиком (для плейлиста уходит ровно один запрос метаданных,
        видео не качаются);
  --ignore-config      — пользовательский конфиг yt-dlp не влияет на разбор;
  --no-warnings, --no-progress — stdout остаётся чистым JSON.

Разбор отчёта: -J печатает JSON в stdout, и он большой (formats, thumbnails,
description, automatic_captions), поэтому в output уходят только нужные поля.
Для URL-плейлиста верхнего уровня полей duration/uploader/formats нет — там
duration=0, uploader="", formats=0, а title — имя плейлиста. Если в stdout
оказалось несколько JSON-значений, берётся последний объект.

Выход (stdout JSON): {url, title, uploader, duration, extractor, formats}.
Отчёт нечитаем → bad_report (exit 2), пустой stdout → no_report. Доменные
ошибки: empty_url, bad_url (аргумент начинается с - либо содержит пробелы),
yt_dlp_not_installed, timeout (retryable), no_report, tool_failed. Платформенные
(exit 2): битый JSON входа, playlist не boolean, нечисловой wall_timeout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)
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
    bin_env = os.environ.get("YT_DLP_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("yt-dlp")
    if found:
        return [found]
    return [sys.executable, "-m", "yt_dlp"]


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
    вывод (штатный случай — -J печатает один JSON), иначе вырезаем значения по
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

    playlist = data.get("playlist")
    if playlist is None:
        playlist = False
    if not isinstance(playlist, bool):
        return fail("bad_playlist", "playlist обязан быть boolean", exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += ["-J", "--skip-download", "--ignore-config",
            "--no-warnings", "--no-progress",
            "--yes-playlist" if playlist else "--no-playlist", url]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("yt_dlp_not_installed",
                        "yt-dlp не найден: pip install yt-dlp "
                        "(или укажите YT_DLP_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"yt-dlp не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        blob = stdout + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("yt_dlp_not_installed",
                        "yt-dlp не установлен: pip install yt-dlp "
                        "(или укажите YT_DLP_BIN)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"yt-dlp упал (exit {proc.returncode}): {last}",
                    retryable=bool(RETRY_RE.search(blob)))

    if not stdout.strip():
        return fail("no_report", "yt-dlp не дал JSON в stdout")

    values = json_values(stdout)
    if not values:
        return fail("bad_report", "не разобран JSON от yt-dlp", exit_code=2)
    report = None
    for value in reversed(values):
        if isinstance(value, dict):
            report = value
            break
    if report is None:
        return fail("bad_report", "неожиданный формат JSON от yt-dlp",
                    exit_code=2)

    formats = report.get("formats")
    return ok({
        "url": as_text(report.get("webpage_url")) or url,
        "title": as_text(report.get("title")),
        "uploader": as_text(report.get("uploader") or report.get("channel")),
        "duration": as_number(report.get("duration")),
        "extractor": as_text(report.get("extractor")
                             or report.get("extractor_key")),
        "formats": len(formats) if isinstance(formats, list) else 0,
    })


if __name__ == "__main__":
    sys.exit(main())
