#!/usr/bin/env python3
"""wapiti — сканер уязвимостей веб-приложений (обёртка над CLI wapiti).

Вход (stdin JSON): url (единственная сканируемая цель), module (опц., список
модулей через запятую, дефолт "xss,sql"), wall_timeout (опц., 600).

Вызов: <WAPITI_BIN|wapiti> -u <url> -m <module> --scope url -f json
       -o <временная папка>/wapiti_report.json --store-session <временная папка>/session
       (cwd = временная папка).

--scope url жёстко ограничивает обход единственным URL из входа: ни краулинга
папки, ни расширения на домен — сканируется ровно заданная пользователем цель.
--store-session уводит sqlite-сессию wapiti в ту же временную папку.

Модуль по умолчанию "xss,sql" — неразрушающий аудит. Пресет wapiti `common`
(его собственный дефолт) содержит exec/file/upload, поэтому его не используем.

JSON-репорт wapiti 3.x (wapitiCore/report/jsonreportgenerator.py) — объект с
пятью ключами: classifications, vulnerabilities (словарь «категория → список
находок»), anomalies, additionals, infos. Находка: {method, path, info, level,
parameter, referer, module, http_request, curl_command, wstg} (+ detail при
-dr 2). Разворачиваем в плоский список; пустой раздел vulnerabilities —
нормальный результат (count=0), не ошибка.

Выход (stdout JSON): {url, vulnerabilities[{category, method, path, parameter,
info, module, level}], count}. Доменные ошибки: empty_url, wapiti_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, bad_wall_timeout, битый JSON-отчёт wapiti.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

REPORT_NAME = "wapiti_report.json"
SESSION_DIR = "session"
DEFAULT_MODULE = "xss,sql"
DEFAULT_WALL = 600

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


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
    bin_env = os.environ.get("WAPITI_BIN", "").strip() or "wapiti"
    if "/" in bin_env or "\\" in bin_env:
        cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    return cmd


def as_int(value):
    if isinstance(value, bool) or not isinstance(value, int):
        return 0
    return value


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

    module = str(data.get("module") or "").strip() or DEFAULT_MODULE

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd = resolve_bin()
        cmd += ["-u", url, "-m", module, "--scope", "url",
                "-f", "json", "-o", report_path,
                "--store-session", os.path.join(td, SESSION_DIR)]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("wapiti_not_installed",
                        "wapiti не найден в PATH: pip install wapiti3 "
                        "(или укажите WAPITI_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"wapiti не уложился в {wall:.0f}s: сузьте набор "
                        "модулей или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            lines = (proc.stdout or proc.stderr or "").strip().splitlines()
            return fail("tool_failed",
                        f"wapiti упал (exit {proc.returncode}): "
                        f"{lines[-1] if lines else 'пустой вывод'}")
        if not os.path.isfile(report_path) or os.path.getsize(report_path) == 0:
            return fail("no_report",
                        "wapiti не дал JSON-отчёт (проверьте url и модуль)")
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON wapiti: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report",
                    "неожиданный формат отчёта wapiti (ожидался объект)",
                    exit_code=2)

    found = report.get("vulnerabilities")
    if found is None:
        found = {}
    if not isinstance(found, dict):
        return fail("bad_report",
                    "раздел vulnerabilities отчёта wapiti — не объект",
                    exit_code=2)

    vulnerabilities = []
    for category in sorted(found):
        entries = found[category]
        if not isinstance(entries, list):
            continue
        for entry in entries:
            if not isinstance(entry, dict):
                continue
            vulnerabilities.append({
                "category": str(category),
                "method": str(entry.get("method") or ""),
                "path": str(entry.get("path") or ""),
                "parameter": str(entry.get("parameter") or ""),
                "info": str(entry.get("info") or ""),
                "module": str(entry.get("module") or ""),
                "level": as_int(entry.get("level")),
            })

    return ok({"url": url, "vulnerabilities": vulnerabilities,
               "count": len(vulnerabilities)})


if __name__ == "__main__":
    sys.exit(main())
