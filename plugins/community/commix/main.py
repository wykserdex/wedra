#!/usr/bin/env python3
"""commix — поиск OS-командных инъекций по одной явно заданной цели (CLI commix).

Неинвазивный аудит: сканируется ровно тот url, который дал пользователь — ни
краулинга (--crawl), ни майнинга эндпоинтов, ни файловых техник
(--skip-technique=f), ни эксплуатации (--os-cmd/--os-shell/--file-read/--upload
не передаются). Интерактив запрещён: --batch обязателен.

Вход (stdin JSON): url, parameter (опц.), timeout (опц., с/запрос, 10),
wall_timeout (опц., общий лимит, 300).

Вызов: <COMMIX_BIN|commix> -u <url> --batch --skip-technique=f
       --disable-coloring --timeout N --report-json report.json [-p param]
       (cwd = временная папка). Флага --output-file у commix нет: машинный
       репорт даёт --report-json (commix 4.2+), его JSON-схема — target,
       http_method, command, started, findings[{parameter, http_method,
       technique, type, boundary, payload, reproduce}], finished, requests,
       target_os, waf_detected, evasion_applied.

Выход (stdout JSON): {url, vulnerable, parameter, injections[{parameter,
http_method, technique, type, payload}]}. Отсутствие находок — нормальный
результат (vulnerable=false, injections=[]), а не ошибка. Доменные ошибки:
empty_url, commix_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, битый JSON-репорт commix.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 10
DEFAULT_WALL = 300
REPORT_NAME = "report.json"

try:
    sys.stdin.reconfigure(encoding="utf-8", errors="replace")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


def ok(output):
    print(json.dumps({"status": "ok", "output": output},
                     ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_bin(env_name, default):
    bin_env = os.environ.get(env_name, default).strip() or default
    if "/" in bin_env or "\\" in bin_env:
        return [os.path.abspath(bin_env)]
    return [shutil.which(bin_env) or os.path.abspath(bin_env)]


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
    if not (url.startswith("http://") or url.startswith("https://")):
        return fail("bad_url", "url обязан начинаться с http:// или https://",
                    exit_code=2)

    raw_param = data.get("parameter")
    if raw_param is not None and not isinstance(raw_param, (str, int, float)):
        return fail("bad_parameter", "parameter обязан быть строкой",
                    exit_code=2)
    parameter = str(raw_param or "").strip()

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin("COMMIX_BIN", "commix")
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd += ["-u", url, "--batch", "--skip-technique=f",
                "--disable-coloring", "--timeout", str(int(timeout)),
                "--report-json", report_path]
        if parameter:
            cmd += ["-p", parameter]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("commix_not_installed",
                        "commix не найден: поставьте commix 4.2+ из исходников "
                        "(git clone https://github.com/commixproject/commix) "
                        "и укажите путь в COMMIX_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"commix не уложился в {wall:.0f}s: уменьшите timeout "
                        "на запрос или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        if not os.path.exists(report_path):
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"commix упал с кодом {proc.returncode}: {last}")
            return fail("no_report",
                        f"commix не дал JSON-репорт ({REPORT_NAME}): {last}")
        if os.path.getsize(report_path) == 0:
            return fail("no_report", f"commix дал пустой репорт ({REPORT_NAME})")
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON commix: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат репорта commix",
                    exit_code=2)

    raw_findings = report.get("findings")
    if raw_findings is None:
        raw_findings = []
    if not isinstance(raw_findings, list):
        return fail("bad_report", "findings в репорте commix не массив",
                    exit_code=2)

    injections = []
    for item in raw_findings:
        if not isinstance(item, dict):
            return fail("bad_report", "finding в репорте commix не объект",
                        exit_code=2)
        injections.append({
            "parameter": str(item.get("parameter") or ""),
            "http_method": str(item.get("http_method") or ""),
            "technique": str(item.get("technique") or ""),
            "type": str(item.get("type") or ""),
            "payload": str(item.get("payload") or ""),
        })

    found_param = parameter or next((i["parameter"] for i in injections
                                    if i["parameter"]), "")
    return ok({"url": url, "vulnerable": bool(injections),
               "parameter": found_param, "injections": injections})


if __name__ == "__main__":
    sys.exit(main())