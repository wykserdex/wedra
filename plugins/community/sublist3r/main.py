#!/usr/bin/env python3
"""sublist3r — пассивный сбор поддоменов (обёртка над CLI sublist3r).

Вход (stdin JSON): domain, threads (опц., 30), wall_timeout (опц., 300).

Вызов: <SUBLIST3R_BIN|python3 -m sublist3r> -d <domain> -t <threads> -n
       -o <sublist3r_report.txt>  (cwd = временная папка).

У Sublist3r НЕТ машинного вывода: -o/--output — это имя ТЕКСТОВОГО файла
(write_file пишет по одному поддомену в строку), а не формат «json». Поэтому
берём этот файл; если донор его не создал (в том числе потому что ничего не
нашёл — write_file зовётся только при непустом списке), добираем хосты из
stdout. Оба источника сводятся к списку строк, из которых берём только
имяхосты домена-цели. Пустая находка — ok с пустым массивом.

Выход (stdout JSON): {domain, subdomains[], total}. Доменные ошибки:
empty_domain, bad_domain, sublist3r_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
нечисловые threads/wall_timeout, нечитаемый артефакт отчёта.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_THREADS = 30
DEFAULT_WALL = 300
REPORT_NAME = "sublist3r_report.txt"
HOST_RE = re.compile(r"^[A-Za-z0-9_*][A-Za-z0-9._*-]*\.[A-Za-z0-9-]+$")


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
                                   "temporary", "reset by peer", "resolve",
                                   "max retries"))


def pick_hosts(lines, domain):
    hosts = []
    for raw in lines:
        host = raw.strip().strip(".").lower()
        if not host or not HOST_RE.match(host):
            continue
        if host != domain and not host.endswith("." + domain):
            continue
        if host not in hosts:
            hosts.append(host)
    return hosts


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

    domain = str(data.get("domain") or "").strip().lower()
    if not domain:
        return fail("empty_domain", "domain пуст")
    if (not HOST_RE.match(domain) or "/" in domain or ":" in domain
            or " " in domain):
        return fail("bad_domain", f"это не похоже на домен: {domain!r}")

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

    bin_env = os.environ.get("SUBLIST3R_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
        else:
            bin_env = os.path.abspath(bin_env)
        cmd = [bin_env]
    else:
        cmd = [sys.executable, "-m", "sublist3r"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (прямой exec
        # непереносим: shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += ["-d", domain, "-t", str(threads), "-n", "-o", REPORT_NAME]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("sublist3r_not_installed",
                        "sublist3r не найден: pip install Sublist3r "
                        "(или укажите SUBLIST3R_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"sublist3r не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"sublist3r упал: {last}",
                        retryable=is_transient(proc.stderr))
        stdout = proc.stdout or ""

        report_path = os.path.join(td, REPORT_NAME)
        if os.path.exists(report_path) and not os.path.isfile(report_path):
            return fail("bad_report",
                        f"артефакт {REPORT_NAME} есть, но это не файл",
                        exit_code=2)
        if os.path.isfile(report_path):
            try:
                with open(report_path, encoding="utf-8",
                          errors="replace") as f:
                    lines = f.read().splitlines()
            except OSError as e:
                return fail("bad_report",
                            f"не прочитан отчёт sublist3r: {e}", exit_code=2)
        elif not stdout.strip():
            return fail("no_report",
                        "sublist3r не дал ни файла отчёта, ни вывода")
        elif "Total Unique Subdomains Found" not in stdout:
            return fail("bad_report",
                        "вывод sublist3r не похож на его отчёт: "
                        f"{stdout.strip().splitlines()[-1][:60]!r}", exit_code=2)
        else:
            lines = stdout.splitlines()

    hosts = pick_hosts(lines, domain)
    return ok({"domain": domain, "subdomains": hosts, "total": len(hosts)})


if __name__ == "__main__":
    sys.exit(main())