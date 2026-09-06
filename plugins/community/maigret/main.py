#!/usr/bin/env python3
"""maigret — поиск username по 4000+ сайтам (обёртка над CLI maigret).

Вход (stdin JSON): username, sites[] (опц.), timeout (опц., с/запрос, 10),
wall_timeout (опц., общий лимит, 300).

Вызов: <MAIGRET_BIN|python3 -m maigret> <username> [--site ...] --timeout N
       -J simple --no-color --no-progressbar  (cwd = временная папка).

Maigret 0.6.x кладёт JSON-репорт в reports/report_<username>_simple.json
(относительно CWD) и печатает текстовый репорт в stderr — мы берём файл.

Выход (stdout JSON): {username, found[{site,url,username}], checked, claimed}.
found — только Claimed; пустой репорт (ник не найден) — нормальный результат,
claimed=0. Доменные ошибки: empty_username, maigret_not_installed, timeout
(retryable), no_report. Платформенные (exit 2): битый JSON входа, битый
репорт maigret.
"""
import glob
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 10
DEFAULT_WALL = 300


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def main():
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
        sites = [str(s) for s in sites]

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом", exit_code=2)

    bin_env = os.environ.get("MAIGRET_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищём
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env:
            # имя из PATH; если PATH пуст — относительное от cwd (моки в тестах)
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        else:
            cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "maigret"]
    cmd += [username]
    for s in sites or []:
        cmd += ["--site", s]
    cmd += ["--timeout", str(int(timeout)),
            "-J", "simple", "--no-color", "--no-progressbar"]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("maigret_not_installed",
                        "maigret не найден: pip install maigret "
                        "(или укажите MAIGRET_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"maigret не уложился в {wall:.0f}s: уменьшите sites "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        matches = sorted(glob.glob(os.path.join(td, "reports",
                                                "report_*_simple.json")))
        if not matches:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("no_report",
                        f"maigret не дал JSON-репорт: {last}")
        # один username — берём репорт по нему, иначе последний
        mine = [m for m in matches if username in os.path.basename(m)]
        report_path = (mine or matches)[-1]
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON maigret: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат репорта maigret",
                    exit_code=2)

    found = []
    checked = 0
    for site, info in report.items():
        checked += 1
        st = (info or {}).get("status") or {}
        if st.get("status") == "Claimed":
            found.append({
                "site": site,
                "url": st.get("url") or (info or {}).get("url_user"),
                "username": st.get("username"),
            })

    return ok({"username": username, "found": found,
               "checked": checked, "claimed": len(found)})


if __name__ == "__main__":
    sys.exit(main())
