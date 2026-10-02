#!/usr/bin/env python3
"""degoogle — Google-доркинг по домену (обёртка над CLI degoogle).

Вход (stdin JSON): domain, query (опц., свой дорк), pages (опц., 1),
wall_timeout (опц., общий лимит, 300).

Вызов: <DEGOOGLE_BIN|python3 -m degoogle.degoogle> "<query>" -p <pages>
       (cwd = временная папка).

degoogle 1.0.x не умеет JSON: отчёт идёт текстом в stdout. Формат ровно такой
(см. main() донора): первая строка «-- N results --», пустая, затем блоки
«описание, URL» через пустую строку; если результатов нет — одна строка
«no results». Разбираем этот текст (паттерн B) и режем email/телефоны
регексом по описаниям и URL. Телефоном считаем только «+…» или ≥9 цифр,
чтоб даты вида 2019-05-06 в URL не ловились.

Выход (stdout JSON): {domain, emails[], phones[], checked}. checked —
сколько результатов выдачи разобрано. Пустая выдача — ok с пустыми
массивами. Доменные ошибки: empty_domain, degoogle_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, нечисловые pages/wall_timeout, отчёт не в формате degoogle.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_PAGES = 1
DEFAULT_WALL = 300

HEADER_RE = re.compile(r"^--\s*(\d+)\s+results\s*--$")
EMAIL_RE = re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")
PHONE_RE = re.compile(r"(?<![\w.])\+?\d[\d().\s-]{6,20}\d(?![\w.])")


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
                                   "temporary", "reset by peer", "resolve"))


def parse_results(stdout):
    """Текстовый отчёт degoogle -> [(desc, url)]. Бросает ValueError, если
    это не отчёт degoogle (платформенная ошибка bad_report)."""
    lines = stdout.splitlines()
    idx = 0
    while idx < len(lines) and not lines[idx].strip():
        idx += 1
    if idx >= len(lines):
        return None
    head = lines[idx].strip()
    if head == "no results":
        return []
    if not HEADER_RE.match(head):
        raise ValueError(f"не распознан заголовок отчёта: {head[:60]!r}")
    blocks = []
    current = []
    for raw in lines[idx + 1:]:
        line = raw.rstrip()
        if not line.strip():
            if current:
                blocks.append(current)
                current = []
            continue
        current.append(line.strip())
    if current:
        blocks.append(current)
    results = []
    for block in blocks:
        url = ""
        desc = []
        for line in block:
            if not url and line.startswith("http"):
                url = line
            else:
                desc.append(line)
        if url:
            results.append((" ".join(desc), url))
    return results


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

    query = str(data.get("query") or "").strip() or f'"{domain}"'

    try:
        pages = int(data.get("pages") or DEFAULT_PAGES)
    except (TypeError, ValueError):
        return fail("bad_pages", "pages обязан быть целым числом", exit_code=2)
    if pages < 1:
        return fail("bad_pages", "pages обязан быть >= 1", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("DEGOOGLE_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
        else:
            bin_env = os.path.abspath(bin_env)
        cmd = [bin_env]
    else:
        cmd = [sys.executable, "-m", "degoogle.degoogle"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (прямой exec
        # непереносим: shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += [query, "-p", str(pages)]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("degoogle_not_installed",
                        "degoogle не найден: pip install degoogle "
                        "(или укажите DEGOOGLE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"degoogle не уложился в {wall:.0f}s: уменьшите pages "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"degoogle упал: {last}",
                        retryable=is_transient(proc.stderr))
        stdout = proc.stdout or ""

    if not stdout.strip():
        return fail("no_report", "degoogle не напечатал отчёт")

    try:
        results = parse_results(stdout)
    except ValueError as e:
        return fail("bad_report", str(e), exit_code=2)

    emails = []
    phones = []
    for desc, url in results or []:
        for addr in EMAIL_RE.findall(desc + " " + url):
            addr = addr.strip(".").lower()
            if addr not in emails:
                emails.append(addr)
        for raw in PHONE_RE.findall(desc + " " + url):
            digits = sum(ch.isdigit() for ch in raw)
            if not raw.startswith("+") and digits < 9:
                continue
            phone = re.sub(r"\s+", " ", raw).strip(" .-")
            if len(phone) >= 6 and phone not in phones:
                phones.append(phone)

    return ok({"domain": domain, "emails": emails, "phones": phones,
               "checked": len(results or [])})


if __name__ == "__main__":
    sys.exit(main())