#!/usr/bin/env python3
"""altdns — генерация перестановок поддоменов по словарю (обёртка над CLI).

Вход (stdin JSON): host, words[] (опц.), number_suffix (опц., bool),
threads (опц.), wall_timeout (опц., 300).

Вызов: <ALTDNS_BIN|python3 -m altdns> -i <tmp>/subdomains.txt
       -o <tmp>/permutations.txt -w <tmp>/words.txt [-n] [-t N]
       (cwd = временная папка).

altdns требует файлы на входе, поэтому обёртка сама готовит -i (одна строка —
host) и -w (словарь слов) во временном каталоге. Машинного формата у донора
нет: -o это обычный текст, по одному перестановочному имени в строке, а сам
altdns дедуплицирует через set() — порядок строк не детерминирован, поэтому
выход нормализуется (дедуп + сортировка). Пустой файл -o — нормальный
результат (permutations: [], total: 0), а не ошибка. Резолв (-r/-s) не
включается: плагин остаётся полностью офлайн.

Выход (stdout JSON): {host, permutations[], total}.

Доменные ошибки: empty_host, altdns_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, words не
массив, threads/wall_timeout не числа, нечитаемый отчёт (не UTF-8).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300
DEFAULT_WORDS = ["admin", "api", "beta", "demo", "dev", "qa", "stage",
                 "test"]


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def read_lines(path):
    with open(path, encoding="utf-8") as f:
        return [line.strip() for line in f if line.strip()]


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

    host = str(data.get("host") or "").strip().lower().rstrip(".")
    if not host:
        return fail("empty_host", "host пуст")

    words = data.get("words")
    if words is None:
        words = list(DEFAULT_WORDS)
    if not isinstance(words, list):
        return fail("bad_words", "words обязан быть массивом", exit_code=2)
    words = [str(w).strip().lower() for w in words if str(w).strip()]

    number_suffix = data.get("number_suffix")
    if number_suffix is None:
        number_suffix = False
    if not isinstance(number_suffix, bool):
        return fail("bad_number_suffix", "number_suffix обязан быть bool",
                    exit_code=2)

    threads = data.get("threads")
    if threads is not None:
        try:
            threads = int(float(threads))
        except (TypeError, ValueError):
            return fail("bad_threads", "threads обязан быть числом", exit_code=2)
        if threads < 0:
            return fail("bad_threads", "threads не может быть отрицательным",
                        exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("ALTDNS_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "altdns"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        subdomains_path = os.path.join(td, "subdomains.txt")
        words_path = os.path.join(td, "words.txt")
        out_path = os.path.join(td, "permutations.txt")
        with open(subdomains_path, "w", encoding="utf-8") as f:
            f.write(host + "\n")
        with open(words_path, "w", encoding="utf-8") as f:
            f.write("\n".join(words) + ("\n" if words else ""))

        cmd += ["-i", subdomains_path, "-o", out_path, "-w", words_path]
        if number_suffix:
            cmd += ["-n"]
        if threads is not None:
            cmd += ["-t", str(threads)]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("altdns_not_installed",
                        "altdns не найден: pip install py-altdns "
                        "(или укажите ALTDNS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"altdns не уложился в {wall:.0f}s: уменьшите words "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.stdout:
            sys.stderr.write(proc.stdout)

        if not os.path.exists(out_path):
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else ""
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"altdns упал с кодом {proc.returncode}: {last}")
            return fail("no_report",
                        f"altdns не дал файл перестановок: {last or 'нет -o'}")
        try:
            lines = read_lines(out_path)
        except Exception as e:
            return fail("bad_report", f"не прочитан отчёт altdns: {e}",
                        exit_code=2)

    return ok({"host": host, "permutations": sorted(set(lines)),
               "total": len(set(lines))})


if __name__ == "__main__":
    sys.exit(main())