#!/usr/bin/env python3
"""hakrawler — веб-краулер для OSINT (обёртка над CLI hakrawler).

Вход (stdin JSON): url, depth (опц., 2), threads (опц., 8), subs (опц., bool),
wall_timeout (опц., общий лимит, 300).

Вызов: <HAKRAWLER_BIN|hakrawler> -json -u -t N -d M [-subs]   (cwd = временная
папка; stdin закрыт после передачи цели — «cat urls.txt | hakrawler» — если
stdin опустеет, hakrawler не напечатает «No urls detected», а сразу выйдет).

Флаги сверены с hakrawler.go (hakluke/hakrawler v2): URL берутся ТОЛЬКО из
stdin (флага -url в v2 нет), -d глубина, -t потоки, -u только уникальные,
-s/-w показать источник/где найдено, -subs субдомены, -json вывод в JSON.
Отдельного флага --complete в v2 нет; машинный формат — -json.

Вывод: по строке на находку. С -json это объект с полями Go-структуры без
тегов — {"Source":"href","URL":"https://...","Where":""} (Where пустой, пока
не задан -w). Без -json была бы голая ссылка.

Выход (stdout JSON): {url, results[{url,source,where}], count}; results
отсортированы по url и дедуплицированы. Пустой stdout — нормальный результат
(цель ничего не отдала), это НЕ ошибка.
Доменные ошибки: empty_url, bad_url, hakrawler_not_installed, timeout
(retryable), tool_failed. Платформенные (exit 2): битый JSON входа,
нечитаемая строка вывода (bad_report).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

DEFAULT_DEPTH = 2
DEFAULT_THREADS = 8
DEFAULT_WALL = 300


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def as_bool(value, default):
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        return value.strip().lower() in ("1", "true", "yes", "on")
    return default


def int_field(data, name, default, code):
    """(значение, код ошибки): пустое поле → default, не число → код."""
    raw = data.get(name)
    if raw is None or raw == "":
        return default, None
    try:
        return int(float(raw)), None
    except (TypeError, ValueError):
        return None, code


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
    if "://" not in url:
        url = "https://" + url
    if not url.startswith(("http://", "https://")):
        return fail("bad_url", f"схема не http(s): {url}")

    depth, bad = int_field(data, "depth", DEFAULT_DEPTH, "bad_depth")
    if bad:
        return fail(bad, "depth обязан быть числом")
    if depth < 1:
        return fail("bad_depth", "depth должен быть >= 1")
    threads, bad = int_field(data, "threads", DEFAULT_THREADS, "bad_threads")
    if bad:
        return fail(bad, "threads обязан быть числом")
    if threads < 1:
        return fail("bad_threads", "threads должен быть >= 1")
    subs = as_bool(data.get("subs"), False)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("HAKRAWLER_BIN", "hakrawler").strip()
    # subprocess поедет с cwd во временную папку: имя из PATH ищем which'ем,
    # путь — приводим к абсолютному
    if "/" not in bin_env and "\\" not in bin_env:
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)
    if bin_env.lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable, bin_env]
    else:
        cmd = [bin_env]
    cmd += ["-json", "-u", "-t", str(threads), "-d", str(depth)]
    if subs:
        cmd.append("-subs")

    with tempfile.TemporaryDirectory() as td:
        try:
            # input= → stdin закрыт после цели: hakrawler читает построчно
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  input=url + "\n")
        except FileNotFoundError:
            return fail("hakrawler_not_installed",
                        "hakrawler не найден: go install "
                        "github.com/hakluke/hakrawler@latest "
                        "(или укажите HAKRAWLER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"hakrawler не уложился в {wall:.0f}s: уменьшите "
                        "depth/threads или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        raw_out = proc.stdout

    if proc.returncode != 0:
        tail = (proc.stderr or raw_out or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed", f"hakrawler упал (exit {proc.returncode}): "
                                   f"{last}")

    results = []
    seen = set()
    for line in raw_out.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            if line.startswith(("http://", "https://")):
                # hakrawler без -json печатает голую ссылку — на всякий случай
                found, source, where = line, "", ""
            else:
                record = json.loads(line)
                if not isinstance(record, dict):
                    return fail("bad_report",
                                "строка вывода hakrawler не объект: "
                                + line[:120], exit_code=2)
                # Go-структура без json-тегов: Source/URL/Where с заглавной
                found = str(record.get("URL") or record.get("url") or "").strip()
                source = record.get("Source", record.get("source"))
                where = record.get("Where", record.get("where"))
                if not found:
                    return fail("bad_report",
                                "в строке вывода hakrawler нет поля URL: "
                                + line[:120], exit_code=2)
        except ValueError as e:
            return fail("bad_report",
                        f"строка вывода hakrawler не JSON ({e}): {line[:120]}",
                        exit_code=2)
        if found in seen:
            continue
        seen.add(found)
        results.append({"url": found,
                        "source": str(source or ""),
                        "where": str(where or "")})

    results.sort(key=lambda item: item["url"])
    return ok({"url": url, "results": results, "count": len(results)})


if __name__ == "__main__":
    sys.exit(main())