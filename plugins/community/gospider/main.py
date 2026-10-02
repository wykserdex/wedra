#!/usr/bin/env python3
"""gospider — краулер с извлечением JS-эндпоинтов (обёртка над CLI gospider).

Вход (stdin JSON): url, depth (опц., 1), concurrent (опц., 5), threads (опц., 1),
sitemap (опц., bool), js (опц., bool, true), cookie (опц.), wall_timeout
(опц., общий лимит, 300).

Вызов: <GOSPIDER_BIN|gospider> -s <url> -o <tmp/out> --json -d N -c N -t N
       [--sitemap] [--js|--js=false] [--cookie C]  (cwd = временная папка,
       stdin = пустой канал: gospider умеет читать список целей из stdin, нам
       это не нужно).

Флаги сверены с main.go gospider v1.1.6 (jaeles-project/gospider): -s/--site,
-o/--output (это ПАПКА), -t/--threads, -c/--concurrent (параллельные запросы на
домен, а НЕ cookie — cookie это длинный флаг --cookie), -d/--depth,
--js (linkfinder по JS), --sitemap, --cookie, --json.

Отчёт: gospider пишет в папку -o файл с именем хоста, где точки заменены на
подчёркивания (core/crawler.go: NewOutput(folder, hostname.replace(".","_"))).
С --json каждая строка файла — объект SpiderOutput {input, source, type,
output, status, length}; type бывает url / javascript / linkfinder / form /
upload-form / subdomain / aws.

Выход (stdout JSON): {url, endpoints[{url,type,source,status}], count};
endpoints отсортированы по (type, url) и дедуплицированы — отчёт воспроизводим.
Доменные ошибки: empty_url, bad_url, gospider_not_installed, timeout
(retryable), no_report. Платформенные (exit 2): битый JSON входа, нечитаемый
отчёт (bad_report).
"""
import glob
import json
import os
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

DEFAULT_DEPTH = 1
DEFAULT_CONCURRENT = 5
DEFAULT_THREADS = 1
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


def as_int(value, default):
    try:
        return int(float(value))
    except (TypeError, ValueError):
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
    if depth < 0:
        return fail("bad_depth", "depth не может быть отрицательным")
    concurrent, bad = int_field(data, "concurrent", DEFAULT_CONCURRENT,
                                "bad_concurrent")
    if bad:
        return fail(bad, "concurrent обязан быть числом")
    if concurrent < 1:
        return fail("bad_concurrent", "concurrent должен быть >= 1")
    threads, bad = int_field(data, "threads", DEFAULT_THREADS, "bad_threads")
    if bad:
        return fail(bad, "threads обязан быть числом")
    if threads < 1:
        return fail("bad_threads", "threads должен быть >= 1")
    sitemap = as_bool(data.get("sitemap"), False)
    linkfinder = as_bool(data.get("js"), True)
    cookie = str(data.get("cookie") or "").strip()

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("GOSPIDER_BIN", "gospider").strip()
    # subprocess поедет с cwd во временной папке: имя из PATH ищем which'ем,
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

    with tempfile.TemporaryDirectory() as td:
        outdir = os.path.join(td, "out")
        os.makedirs(outdir, exist_ok=True)
        cmd += ["-s", url, "-o", outdir, "--json",
                "-d", str(depth), "-c", str(concurrent), "-t", str(threads)]
        if sitemap:
            cmd.append("--sitemap")
        cmd.append("--js=true" if linkfinder else "--js=false")
        if cookie:
            cmd += ["--cookie", cookie]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("gospider_not_installed",
                        "gospider не найден: go install "
                        "github.com/jaeles-project/gospider@latest "
                        "(или укажите GOSPIDER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"gospider не уложился в {wall:.0f}s: уменьшите "
                        "depth/concurrent или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        report_path = os.path.join(
            outdir, (urlsplit(url).hostname or "").replace(".", "_"))
        if not os.path.isfile(report_path):
            candidates = [p for p in glob.glob(os.path.join(outdir, "*"))
                          if os.path.isfile(p)]
            if not candidates:
                tail = (proc.stderr or proc.stdout or "").strip().splitlines()
                last = tail[-1] if tail else f"exit {proc.returncode}"
                return fail("no_report",
                            f"gospider не дал отчёт в папке -o: {last}")
            report_path = max(candidates, key=os.path.getmtime)

        try:
            with open(report_path, encoding="utf-8") as f:
                raw_lines = f.read().splitlines()
        except Exception as e:
            return fail("bad_report", f"не прочитан отчёт gospider: {e}",
                        exit_code=2)

    endpoints = []
    seen = set()
    for line in raw_lines:
        line = line.strip()
        if not line:
            continue
        try:
            record = json.loads(line)
        except ValueError as e:
            return fail("bad_report",
                        f"строка отчёта gospider не JSON ({e}): {line[:120]}",
                        exit_code=2)
        if not isinstance(record, dict):
            return fail("bad_report",
                        "строка отчёта gospider не объект: " + line[:120],
                        exit_code=2)
        found = str(record.get("output") or "").strip()
        if not found:
            continue
        kind = str(record.get("type") or "").strip()
        key = (kind, found)
        if key in seen:
            continue
        seen.add(key)
        endpoints.append({
            "url": found,
            "type": kind,
            "source": str(record.get("source") or ""),
            "status": as_int(record.get("status"), 0),
        })

    endpoints.sort(key=lambda item: (item["type"], item["url"]))
    return ok({"url": url, "endpoints": endpoints, "count": len(endpoints)})


if __name__ == "__main__":
    sys.exit(main())