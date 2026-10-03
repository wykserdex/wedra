#!/usr/bin/env python3
"""sherlock — поиск username по 400+ соцсетям (обёртка над CLI sherlock).

Вход (stdin JSON): username, sites[] (опц., --site), timeout (опц., с/сайт,
60), wall_timeout (опц., общий лимит, 600).

Вызов: <SHERLOCK_BIN|python3 -m sherlock_project> <username> [--site S ...]
       --print-all --no-color --timeout N   (cwd = временная папка).

Флаги сверены с исходником пакета sherlock-project 0.16.2
(sherlock_project/sherlock.py, main()): --site (action=append, dest=site_list),
--print-all, --no-color, --timeout (type=timeout_check, default 60). Позиционный
username — nargs="+", поэтому username идёт первым. Без SHERLOCK_BIN модуль
запускается как -m sherlock_project: в wheel верхний уровень — пакет
sherlock_project, а «sherlock» — только имя консольного скрипта из
entry_points (sherlock=sherlock_project.sherlock:main), модуля sherlock нет.
Машинного отчёта не берём намеренно: плагин разбирает stdout (паттерн B) —
[+] Сайт: url считается найденным, [-] Сайт: ... считается проверенным.
Строка-заголовок «[*] Checking username ... on:» нужна как признак того, что
инструмент вообще отработал; без неё и без строк [+] / [-] — no_report.

Выход (stdout JSON): {username, found[{site,url}], checked, claimed}.
checked — сколько строк [+] / [-] в отчёте, claimed = len(found). Пустой
репорт (ник нигде не найден) — нормальный результат: ok с found=[]. Доменные
ошибки: empty_username, sherlock_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, sites не
массив, нечисловые timeout/wall_timeout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 60
DEFAULT_WALL = 600

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
FOUND_RE = re.compile(r"^\s*\[\+\]\s*(.+?):\s*(https?://\S+)\s*$")
CHECKED_RE = re.compile(r"^\s*\[[\+\-]\]\s*\S")
HEADER_RE = re.compile(r"^\s*\[\*\]\s*Checking\s+username\b", re.IGNORECASE)


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("SHERLOCK_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        # имя из PATH; если PATH пуст — относительное от cwd плагина (моки)
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    # имя пакета — sherlock_project (sherlock_project.sherlock:main), а
    # «sherlock» — лишь имя консольного скрипта из entry_points; модуля с
    # таким именем в wheel нет, поэтому -m sherlock не отработает.
    return [sys.executable, "-m", "sherlock_project"]


def parse_report(stdout):
    found = []
    checked = 0
    header = False
    for raw in stdout.splitlines():
        line = ANSI_RE.sub("", raw).rstrip()
        if HEADER_RE.match(line):
            header = True
            continue
        hit = FOUND_RE.match(line)
        if hit:
            checked += 1
            site = hit.group(1).strip()
            if not any(f["site"] == site for f in found):
                found.append({"site": site, "url": hit.group(2).strip()})
            continue
        if CHECKED_RE.match(line):
            checked += 1
    return found, checked, header


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

    username = str(data.get("username") or "").strip()
    if not username:
        return fail("empty_username", "username пуст")

    sites = data.get("sites")
    if sites is not None and not isinstance(sites, list):
        return fail("bad_sites", "sites обязан быть массивом", exit_code=2)
    if isinstance(sites, list):
        sites = [str(s).strip() for s in sites if str(s).strip()]

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += [username]
    for site in sites or []:
        cmd += ["--site", site]
    cmd += ["--print-all", "--no-color", "--timeout", str(int(timeout))]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("sherlock_not_installed",
                        "sherlock не найден: pip install sherlock-project "
                        "(или укажите SHERLOCK_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"sherlock не уложился в {wall:.0f}s: уменьшите sites "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    if proc.returncode != 0:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed", f"sherlock упал (exit {proc.returncode}): {last}")

    found, checked, header = parse_report(proc.stdout or "")
    if not header and not checked:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()
        last = tail[-1] if tail else "пустой stdout"
        return fail("no_report", f"sherlock не дал распознаваемый репорт: {last}")

    return ok({"username": username, "found": found, "checked": checked,
               "claimed": len(found)})


if __name__ == "__main__":
    sys.exit(main())