#!/usr/bin/env python3
"""wafw00f — определение WAF/файрвола за сайтом (обёртка над CLI wafw00f).

Вход (stdin JSON): url, findall (опц., bool, -a), timeout (опц., с/запрос, 7),
wall_timeout (опц., общий лимит, 300).

Вызов: <WAFW00F_BIN|wafw00f|python3 -m wafw00f.main> -o wafw00f_report.json
       [-a] -T N --no-colors <url>  (cwd = временная папка).

Флаги сверены с main.py wafw00f 2.4.2 (optparse, console_scripts
wafw00f = wafw00f.main:main): -o/--output — файл отчёта, формат выбирается по
расширению (.json → JSON), -f/--format — принудительный формат, -a/--findall —
искать все совпадения, -i/--input-file — список целей из файла, -T/--timeout —
таймаут запроса (int, по умолчанию 7), --no-colors. В пакете нет __main__.py,
поэтому модульный запуск только через wafw00f.main. Отчёт — JSON-массив записей
{url, detected, trigger_url, firewall, manufacturer}; при отсутствии WAF
запись {detected: false, firewall: "None", manufacturer: "None"}, а при
общей детекции — {firewall: "Generic", manufacturer: "Unknown"}. В stdout
инструмент печатает баннер, поэтому берём файл, а не stdout.

Выход (stdout JSON): {url, waf, manufacturer, detected}. Взят первый отчёт с
detected=true; «ничего не нашлось» — ok с detected=false и пустыми строками.
Сайт не отвечает — инструмент не пишет файл, это no_report. Доменные ошибки:
empty_url, wafw00f_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечисловые таймауты, неверный тип
findall, нечитаемый отчёт.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 7
DEFAULT_WALL = 300
REPORT_NAME = "wafw00f_report.json"


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
    bin_env = os.environ.get("WAFW00F_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("wafw00f")
    if found:
        return [found]
    return [sys.executable, "-m", "wafw00f.main"]


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

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")

    findall = data.get("findall")
    if findall is None:
        findall = False
    elif not isinstance(findall, bool):
        return fail("bad_findall", "findall обязан быть boolean", exit_code=2)

    try:
        timeout = int(float(data.get("timeout") or DEFAULT_TIMEOUT))
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
    cmd += ["-o", REPORT_NAME]
    if findall:
        cmd += ["-a"]
    cmd += ["-T", str(timeout), "--no-colors", url]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("wafw00f_not_installed",
                        "wafw00f не найден: pip install wafw00f "
                        "(или укажите WAFW00F_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"wafw00f не уложился в {wall:.0f}s: уменьшите "
                        "findall или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        report_path = os.path.join(td, REPORT_NAME)
        if not os.path.isfile(report_path):
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"wafw00f упал (exit {proc.returncode}): {last}")
            return fail("no_report",
                        f"wafw00f не дал JSON-отчёт (сайт не отвечает?): {last}")
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"wafw00f упал (exit {proc.returncode}): {last}")
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON wafw00f: {e}",
                        exit_code=2)

    if not isinstance(report, list):
        return fail("bad_report",
                    "ожидался массив записей в отчёте wafw00f", exit_code=2)

    waf = ""
    manufacturer = ""
    detected = False
    for record in report:
        if not isinstance(record, dict) or detected:
            continue
        if record.get("detected"):
            detected = True
            waf = str(record.get("firewall") or "")
            manufacturer = str(record.get("manufacturer") or "")

    return ok({"url": url, "waf": waf, "manufacturer": manufacturer,
               "detected": detected})


if __name__ == "__main__":
    sys.exit(main())
