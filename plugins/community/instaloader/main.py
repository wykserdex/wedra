#!/usr/bin/env python3
"""instaloader — метаданные публичного профиля Instagram (обёртка, паттерн C).

Вход (stdin JSON): profile (обязателен, имя профиля без @), wall_timeout (опц.,
общий лимит, 300).

Вызов: <INSTALOADER_BIN|...> либо `python3 -c <SNIPPET> <profile>`
       (cwd = временная папка). Почему не CLI. Сверено с instaloader 4.15.3
       (instaloader/__main__.py, instaloader/instaloader.py,
       instaloader/structures.py, instaloader/instaloadercontext.py): флага
       `--json` (JSON в stdout) в CLI нет вообще — есть позиционный аргумент
       `json`, но это файл со списком целей, а не флаг; `--no-login` нет тоже —
       есть только `--no-metadata-json`, и он отключает запись JSON-файлов
       метаданных постов. Метаданные профиля CLI наружу не отдаёт, поэтому
       продовый путь — паттерн C: тот же интерпретатор и фиксированный сниппет
       на публичном API instaloader, который печатает JSON в stdout.

Логин не используется. Ник, пароль и cookie-файл плагин не передаёт и не
читает: InstaloaderContext поднимает анонимную сессию (get_anonymous_session,
username=None — instaloader/instaloadercontext.py), а логинится только при
явном InstaloaderContext.login(user, passwd); Profile.from_username работает без
логина (профиль закрытый вернёт is_private с нулевыми счётчиками). Медиа не
качается: в сниппете download_pictures/download_videos/
download_video_thumbnails=False и save_metadata=False — те же выключатели, что у
флагов --no-pictures, --no-videos, --no-video-thumbnails, --no-metadata-json
(сверено с :param:-описанием Instaloader.__init__ в instaloader/instaloader.py).

Сниппет читает у Profile только те свойства, которые реально нужны выходу.
Все они — @property поверх Profile._metadata() и на нормальном узле
web_profile_info безопасны: username, full_name, followers, followees,
mediacount, is_private. Свойства biography и profile_pic_url из сниппета
убраны намеренно: они в выход не идут, но могут уронить весь прогон —
biography зовёт normalize("NFC", node['biography']) и падает TypeError на
биографии null, а profile_pic_url читает ключ profile_pic_url_hd, которого
Profile.from_username не добавляет (нормализацию _normalize_profile_data
вызывает только _obtain_metadata), и падает KeyError.

Выход (stdout JSON): {profile, username, followers, posts, is_private,
full_name]. Сниппет — константа модуля, секретов в ней нет и быть не может:
логин не используется. Профиль не найден или закрыт и без логина недоступен —
instaloader падает с ненулевым кодом, это tool_failed (если текст ошибки про
429/5xx/таймаут — retryable). Пустой stdout — no_report, нечитаемый JSON —
bad_report (exit 2). В stdout ожидается один JSON-объект; если объектов
несколько, берётся последний.
Доменные ошибки: empty_profile, bad_profile (не имя профиля), instaloader_not_
installed, timeout (retryable), no_report, tool_failed. Платформенные (exit 2):
битый JSON входа, нечисловой wall_timeout, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

PROFILE_RE = re.compile(r"^[A-Za-z0-9._]+$")
NOT_MODULE_RE = re.compile(r"No module named|ModuleNotFoundError",
                           re.IGNORECASE)
RETRY_RE = re.compile(
    r"HTTP Error (429|5\d\d)|Too Many Requests|timed out|connection reset|"
    r"connection aborted|Temporary failure|temporarily unavailable",
    re.IGNORECASE,
)

SNIPPET = (
    "import json,sys,instaloader as il;"
    "L=il.Instaloader(quiet=True,sleep=False,download_pictures=False,"
    "download_videos=False,download_video_thumbnails=False,save_metadata=False,"
    "compress_json=False);"
    "p=il.Profile.from_username(L.context,sys.argv[1]);"
    "n=lambda v:int(v or 0);"
    "print(json.dumps({'username':p.username or '',"
    "'full_name':p.full_name or '',"
    "'followers':n(p.followers),'followees':n(p.followees),"
    "'posts':n(p.mediacount),'is_private':bool(p.is_private)},"
    "ensure_ascii=False))"
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
    bin_env = os.environ.get("INSTALOADER_BIN", "").strip()
    if not bin_env:
        return [sys.executable, "-c", SNIPPET]
    if "/" in bin_env or "\\" in bin_env:
        cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    return cmd


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

    profile = str(data.get("profile") or "").strip().lstrip("@")
    if not profile:
        return fail("empty_profile", "profile пуст")
    if not PROFILE_RE.match(profile):
        return fail("bad_profile",
                    "profile — имя профиля Instagram: латиница, цифры, точка, "
                    "подчёркивание (до 30 символов)")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    cmd += [profile]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("instaloader_not_installed",
                        "instaloader не найден: pip install instaloader "
                        "(или укажите INSTALOADER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"instaloader не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        blob = stdout + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("instaloader_not_installed",
                        "instaloader не установлен: pip install instaloader "
                        "(или укажите INSTALOADER_BIN)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"instaloader упал (exit {proc.returncode}): {last}",
                    retryable=bool(RETRY_RE.search(blob)))

    if not stdout.strip():
        return fail("no_report", "instaloader не дал JSON в stdout")

    values = json_values(stdout)
    report = None
    for value in reversed(values):
        if isinstance(value, dict):
            report = value
            break
    if report is None:
        return fail("bad_report", "не разобран JSON от instaloader",
                    exit_code=2)

    return ok({
        "profile": profile,
        "username": as_text(report.get("username")) or profile,
        "followers": as_number(report.get("followers")),
        "posts": as_number(report.get("posts")),
        "is_private": bool(report.get("is_private")),
        "full_name": as_text(report.get("full_name")),
    })


if __name__ == "__main__":
    sys.exit(main())
