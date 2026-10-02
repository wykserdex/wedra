#!/usr/bin/env python3
"""metafinder — поиск метаданных в выдаче поисковиков (обёртка над CLI metafinder).

Вход (stdin JSON): domain, limit (опц., 20), threads (опц., 4),
engines (опц., ["google","bing","baidu"]), wall_timeout (опц., 300).

Вызов: <METAFINDER_BIN|python3 -m metafinder.cli> -d <domain> -o report
       -l <limit> -t <threads> [-go] [-bi] [-ba]  (cwd = временная папка).

Отчёта в stdout нет: metafinder 1.2 кладёт текстовый репорт в
<report>/<domain>/metadata_result.txt (соседние authors.txt/software.txt —
подмножества тех же данных). Формат блока (utils/file/parser.py: пустая
строка, имя файла, подчёркивание, затем строки):

    URL: <url>
    Status code: <code>
    Search engines: <через запятую>
    |_ Author: <значение>          (либо «|_ No metadata found»)

Разбираем эти блоки (паттерн A + осторожный текст). checked — сколько
документов в отчёте, results — только те, у которых метаданные есть
(без строки «No metadata found»). Пустой отчёт = ok с пустым массивом.

Выход (stdout JSON): {domain, results[{source,url,title}], checked}.
Доменные ошибки: empty_domain, bad_engines, metafinder_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
нечисловые limit/threads/wall_timeout/engines, нечитаемый отчёт.
"""
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_LIMIT = 20
DEFAULT_THREADS = 4
DEFAULT_WALL = 300
REPORT_DIR = "report"
ENGINE_FLAGS = {"google": "-go", "bing": "-bi", "baidu": "-ba"}


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def is_transient(stderr):
    tail = (stderr or "").lower()
    return any(w in tail for w in ("timeout", "timed out", "connection",
                                   "temporary", "reset by peer", "max retries"))


def parse_report(text):
    """Текстовый отчёт metafinder -> ([все блоки], [блоки с метаданными])."""
    blocks = []
    with_meta = []
    for raw in re.split(r"\n\s*\n", text):
        lines = [line.strip() for line in raw.splitlines() if line.strip()]
        if len(lines) < 2 or set(lines[1]) != {"-"}:
            continue
        item = {"source": "", "url": "", "title": lines[0]}
        has_meta = False
        for line in lines[2:]:
            if line.startswith("URL: "):
                item["url"] = line[5:].strip()
            elif line.startswith("Search engines: "):
                item["source"] = line[16:].strip()
            elif line.startswith("|_ ") and line[3:] != "No metadata found":
                has_meta = True
        blocks.append(item)
        if has_meta:
            with_meta.append(item)
    return blocks, with_meta


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8")
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    domain = str(data.get("domain") or "").strip()
    if not domain:
        return fail("empty_domain", "domain пуст")

    engines = data.get("engines")
    if engines is not None and not isinstance(engines, list):
        return fail("bad_engines", "engines обязан быть массивом", exit_code=2)
    picked = []
    for name in engines or []:
        key = str(name).strip().lower()
        if key not in ENGINE_FLAGS:
            return fail("bad_engines",
                        f"неизвестный поисковик {name!r}: "
                        f"допустимо {', '.join(sorted(ENGINE_FLAGS))}")
        if key not in picked:
            picked.append(key)

    try:
        limit = int(data.get("limit") or DEFAULT_LIMIT)
    except (TypeError, ValueError):
        return fail("bad_limit", "limit обязан быть целым числом", exit_code=2)
    if limit < 1:
        return fail("bad_limit", "limit обязан быть >= 1", exit_code=2)
    try:
        threads = int(data.get("threads") or DEFAULT_THREADS)
    except (TypeError, ValueError):
        return fail("bad_threads", "threads обязан быть целым числом",
                    exit_code=2)
    if threads < 1:
        return fail("bad_threads", "threads обязан быть >= 1", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("METAFINDER_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
        else:
            bin_env = os.path.abspath(bin_env)
        cmd = [bin_env]
    else:
        cmd = [sys.executable, "-m", "metafinder.cli"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (прямой exec
        # непереносим: shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += ["-d", domain, "-o", REPORT_DIR, "-l", str(limit),
            "-t", str(threads)]
    for name in picked:
        cmd.append(ENGINE_FLAGS[name])

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("metafinder_not_installed",
                        "metafinder не найден: pip install metafinder "
                        "(или укажите METAFINDER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"metafinder не уложился в {wall:.0f}s: уменьшите limit "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"metafinder упал: {last}",
                        retryable=is_transient(proc.stderr))

        matches = sorted(glob.glob(os.path.join(
            td, "**", "metadata_result.txt"), recursive=True))
        if not matches:
            # metafinder пишет отчёт только когда метаданные нашлись; его
            # собственные строки про это знаем однозначно — это не сбой.
            quiet = ("No metadata found" in stdout
                     or "There is nothing to analyze" in stdout)
            if quiet:
                return ok({"domain": domain, "results": [], "checked": 0})
            tail = stdout.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("no_report", f"metafinder не дал отчёт: {last}")
        try:
            with open(matches[-1], encoding="utf-8", errors="replace") as f:
                text = f.read()
        except OSError as e:
            return fail("bad_report", f"не прочитан отчёт metafinder: {e}",
                        exit_code=2)

    if not text.strip():
        return ok({"domain": domain, "results": [], "checked": 0})

    blocks, results = parse_report(text)
    if not blocks:
        return fail("bad_report",
                    "отчёт metafinder без распознанных блоков "
                    "(формат изменился?)", exit_code=2)

    return ok({"domain": domain, "results": results, "checked": len(blocks)})


if __name__ == "__main__":
    sys.exit(main())