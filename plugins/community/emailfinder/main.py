#!/usr/bin/env python3
"""emailfinder — email-адреса домена через Hunter.io (обёртка над CLI).

Вход (stdin JSON): domain, wall_timeout (опц., общий лимит, 300).

Вызов: <EMAILFINDER_BIN|emailfinder|python3 -m emailfinder.cli> -d <домен>
       (cwd = временная папка).

CLI сверен с исходником пакета `pip install emailfinder` (josue87/emailfinder,
репозиторий soxoj/emailfinder) 0.3.0b0, файл emailfinder/cli.py: единственный
обязательный флаг поиска — `-d/--domain` (required=True), есть ещё
`-p/--proxy` и `-v/--version`. Флагов JSON-вывода у пакета нет, поэтому отчёт
разбирается как текст (паттерн B): по всему stdout ищутся адреса регуляркой.
Без EMAILFINDER_BIN модуль запускается как -m emailfinder.cli: в пакете нет
__main__.py, а entry_points указывает на emailfinder.cli:main, поэтому
-m emailfinder не отработает.
Адреса на целевом домене идут в emails; если таких нет — берём все найденные
(поисковики любят отдавать чужие домены). checked — сколько адресных строк
разобрано из вывода до дедупа.

Ключ Hunter.io нужен инструменту, а не плагину: он читает HUNTER_API_KEY сам
из env, поэтому main.py ключ не проверяет и в манифесте только объявлен.
Плагин в сеть не ходит — вся сеть в дочернем процессе.

Выход (stdout JSON): {domain, emails[], checked}. Инструмент отработал, но
адресов нет — ok с пустым массивом; пустой stdout — no_report. Доменные
ошибки: empty_domain, emailfinder_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, нечисловой
wall_timeout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

EMAIL_RE = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)+")
NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("EMAILFINDER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("emailfinder")
    if found:
        return [found]
    # console_scripts: emailfinder = emailfinder.cli:main; __main__.py в пакете
    # нет, поэтому -m emailfinder не работает — запускаем модуль cli.
    return [sys.executable, "-m", "emailfinder.cli"]


def normalize_domain(raw):
    value = raw.strip().lower()
    for scheme in ("http://", "https://"):
        if value.startswith(scheme):
            value = value[len(scheme):]
    value = value.split("/", 1)[0].split("?", 1)[0].strip()
    if value.startswith("www."):
        value = value[4:]
    return value


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

    domain = normalize_domain(str(data.get("domain") or ""))
    if not domain:
        return fail("empty_domain", "domain пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += ["-d", domain]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("emailfinder_not_installed",
                        "emailfinder не найден: pip install emailfinder "
                        "(или укажите EMAILFINDER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"emailfinder не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    stdout = proc.stdout or ""
    blob = stdout + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("emailfinder_not_installed",
                        "emailfinder не установлен: pip install emailfinder "
                        "(или укажите EMAILFINDER_BIN)")
        tail = (blob or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"emailfinder упал (exit {proc.returncode}): {last}")

    if not stdout.strip():
        return fail("no_report", "emailfinder не дал вывода")

    hits = []
    for match in EMAIL_RE.findall(stdout):
        hits.append(match.strip().rstrip(".").lower())

    own = []
    for mail in hits:
        if mail.rsplit("@", 1)[-1] == domain and mail not in own:
            own.append(mail)
    emails = own or []
    if not emails:
        for mail in hits:
            if mail not in emails:
                emails.append(mail)

    return ok({"domain": domain, "emails": emails, "checked": len(hits)})


if __name__ == "__main__":
    sys.exit(main())