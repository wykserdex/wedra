#!/usr/bin/env python3
"""linkfinder — эндпоинты и ссылки из JavaScript (обёртка над скриптом linkfinder.py).

Вход (stdin JSON): url, crawl (опц., bool, -d), regex (опц., -r), timeout
(опц., с/запрос, 10), wall_timeout (опц., общий лимит, 300).

Вызов: <LINKFINDER_BIN|linkfinder.py> -i <url> -o cli [-d] [-r <regex>]
       -t N  (cwd = временная папка; при -o cli инструмент ничего не пишет
       на диск — весь результат идёт в stdout).

Флаги сверены с linkfinder.py (GerbenJavado/LinkFinder): -i/--input — URL,
файл или папка (обязателен), -o/--output — "cli" печатает список находок в
STDOUT (дефолт — HTML-файл), -d/--domain — обойти все JS домена, -r/--regex —
фильтр, -t/--timeout — таймаут запроса. На PyPI пакета linkfinder нет (404),
поэтому ставится из репозитория; продакшн-путь — путь к linkfinder.py.

Парсинг stdout построчный: строки-разделители самого инструмента
("Usage: ", "Error: ", "Running against: ") отбрасываются, остальное — находки;
дубликаты снимаются с сохранением порядка. Пустой stdout — это «ссылок не
нашлось» (ok, links: []), а не ошибка. Важно: linkfinder сообщает о своих
ошибках самой строкой "Error:" и при этом выходит с кодом 0, поэтому такой
вывод — tool_failed, иначе тихий сбой прошёл бы как успех.

Выход (stdout JSON): {url, links[], count}. Доменные ошибки:
empty_url, linkfinder_not_installed, timeout (retryable), tool_failed.
Платформенные (exit 2): битый JSON входа, неверные типы полей.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 10
DEFAULT_WALL = 300

NO_MODULE_RE = re.compile(
    r"No module named|can't open file|No such file or directory",
    re.IGNORECASE)
NOISE_PREFIXES = ("usage:", "error:", "running against:")


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
    bin_env = os.environ.get("LINKFINDER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if cmd[0].lower().endswith(".py"):
            cmd = [sys.executable] + cmd
        return cmd
    found = shutil.which("linkfinder.py") or shutil.which("linkfinder")
    if found:
        return [sys.executable, found]
    return [sys.executable, "-m", "linkfinder"]


def parse_links(stdout):
    links = []
    for raw in (stdout or "").splitlines():
        line = raw.strip()
        if not line or line.lower().startswith(NOISE_PREFIXES):
            continue
        if line not in links:
            links.append(line)
    return links


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

    crawl = data.get("crawl")
    if crawl is None:
        crawl = False
    elif not isinstance(crawl, bool):
        return fail("bad_crawl", "crawl обязан быть boolean", exit_code=2)

    regex = data.get("regex")
    if regex is None:
        regex = ""
    elif not isinstance(regex, str):
        return fail("bad_regex", "regex обязан быть строкой", exit_code=2)
    regex = regex.strip()

    try:
        timeout = int(float(data.get("timeout") or DEFAULT_TIMEOUT))
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    cmd += ["-i", url, "-o", "cli", "-t", str(timeout)]
    if crawl:
        cmd += ["-d"]
    if regex:
        cmd += ["-r", regex]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("linkfinder_not_installed",
                        "linkfinder.py не найден: поставьте LinkFinder из "
                        "репозитория и укажите LINKFINDER_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"linkfinder не уложился в {wall:.0f}s: уменьшите "
                        "crawl или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    blob = (proc.stdout or "") + "\n" + (proc.stderr or "")

    if proc.returncode != 0 or NO_MODULE_RE.search(blob):
        if NO_MODULE_RE.search(blob):
            return fail("linkfinder_not_installed",
                        "linkfinder.py не установлен: git clone "
                        "https://github.com/GerbenJavado/LinkFinder && "
                        "pip install -r requirements.txt (или укажите "
                        "LINKFINDER_BIN путём к linkfinder.py)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"linkfinder упал (exit {proc.returncode}): {last}")

    for raw in (proc.stdout or "").splitlines():
        if raw.strip().lower().startswith("error:"):
            return fail("tool_failed",
                        f"linkfinder сообщил об ошибке: {raw.strip()}")

    links = parse_links(proc.stdout)

    return ok({"url": url, "links": links, "count": len(links)})


if __name__ == "__main__":
    sys.exit(main())
