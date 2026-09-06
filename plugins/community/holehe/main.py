#!/usr/bin/env python3
"""holehe — проверка email/username на утечки по 120+ сайтам (CLI holehe).

Вход (stdin JSON): target, timeout (опц., с/сайт, 10), wall_timeout (опц.,
общий лимит, 300).

Вызов: <HOLEHE_BIN|holehe> <target> -C --no-color -T N  (cwd = временная
папка). holehe 1.6x пишет CSV: holehe_<ts>_<target>_results.csv в CWD;
колонок: name, domain, method, frequent_rate_limit, rateLimit, exists,
emailrecovery, phoneNumber, others. Методов вывода JSON в CLI нет — CSV
единственный детерминированный формат, парсим его.

Выход (stdout JSON): {target, leaks[{site,domain,method}], checked,
rate_limited, phone_numbers[]}. leaks — строки с exists=True; rate_limited —
строки с rateLimit=True (проверить не удалось); phone_numbers — собранные из
колонок phoneNumber. Доменные ошибки: empty_target, holehe_not_installed,
timeout (retryable), no_report. Платформенные (exit 2): битый JSON входа,
битый CSV.
"""
import csv
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

    target = str(data.get("target") or "").strip()
    if not target:
        return fail("empty_target", "target пуст")

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("HOLEHE_BIN", "holehe").strip()
    # subprocess поедет с cwd во временной папке: имя из PATH ищем
    # which'ем, путь — приводим к абсолютному
    if "/" not in bin_env:
        # имя из PATH; если PATH пуст — относительное от cwd (моки в тестах)
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)

    cmd = [bin_env, target, "-C", "--no-color", "-T", str(int(timeout))]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("holehe_not_installed",
                        "holehe не найден в PATH: pip install holehe "
                        "(или укажите HOLEHE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"holehe не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        matches = sorted(glob.glob(os.path.join(td,
                                                "holehe_*_results.csv")))
        if not matches:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("no_report", f"holehe не дал CSV: {last}")
        csv_path = matches[-1]
        try:
            with open(csv_path, newline="", encoding="utf-8") as f:
                rows = list(csv.DictReader(f))
        except Exception as e:
            return fail("bad_report", f"не прочитан CSV holehe: {e}",
                        exit_code=2)

    leaks = []
    rate_limited = 0
    phones = []
    for row in rows:
        if str(row.get("exists", "")).strip().lower() == "true":
            leaks.append({"site": row.get("name"),
                          "domain": row.get("domain"),
                          "method": row.get("method")})
        if str(row.get("rateLimit", "")).strip().lower() == "true":
            rate_limited += 1
        phone = (row.get("phoneNumber") or "").strip()
        if phone and phone not in phones:
            phones.append(phone)

    return ok({"target": target, "leaks": leaks, "checked": len(rows),
               "rate_limited": rate_limited, "phone_numbers": phones})


if __name__ == "__main__":
    sys.exit(main())
