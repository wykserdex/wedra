#!/usr/bin/env python3
"""wfuzz — фаззинг URL/параметров по словарю (обёртка над CLI wfuzz).

Вход (stdin JSON): url (может содержать маркер FUZZ), wordlist (опц.),
matcher (опц., выражение фильтра wfuzz), wall_timeout (опц., 300).

Вызов: <WFUZZ_BIN|python3 -m wfuzz> -u <url> -w <wordlist> [--filter <matcher>]
       -f <временная папка>/wfuzz_report.json,json   (cwd = временная папка).

Wfuzz 2.1.x принтером json пишет в файл из -f один JSON-массив объектов:
[{code, lines, words, chars, payload, location, method, post_data, server, url}].
Берём этот файл. Пустой массив [] — нормальный результат (count=0), не ошибка.
FUZZ — маркер wfuzz: он не «разворачивается» в наш код, а остаётся в URL и
подставляется донором (одна цель — один прогон, никакой самостоятельной разведки).

Выход (stdout JSON): {url, results[{url,payload,code,lines,words,chars}], count}.
Доменные ошибки: empty_url, empty_wordlist, missing_file, wfuzz_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, bad_wall_timeout, битый JSON-репорт wfuzz.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

REPORT_NAME = "wfuzz_report.json"
DEFAULT_WALL = 300

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
    bin_env = os.environ.get("WFUZZ_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "wfuzz"]
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

    wordlist = str(data.get("wordlist") or "").strip()
    if not wordlist:
        return fail("empty_wordlist",
                    "wfuzz без источника payload не запускается: укажите "
                    "wordlist (путь к файлу; относительный — от каталога плагина)")

    matcher = str(data.get("matcher") or "").strip()

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    base = os.path.dirname(os.path.abspath(__file__))
    wordlist_path = wordlist if os.path.isabs(wordlist) else os.path.join(base, wordlist)
    if not os.path.isfile(wordlist_path):
        return fail("missing_file", f"файл словаря не найден: {wordlist}")

    report_path = None
    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd = resolve_bin()
        cmd += ["-u", url, "-w", wordlist_path, "-f", report_path + ",json"]
        if matcher:
            cmd += ["--filter", matcher]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("wfuzz_not_installed",
                        "wfuzz не найден: pip install wfuzz "
                        "(или укажите WFUZZ_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"wfuzz не уложился в {wall:.0f}s: уменьшите словарь "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            lines = (proc.stdout or proc.stderr or "").strip().splitlines()
            return fail("tool_failed",
                        f"wfuzz упал (exit {proc.returncode}): "
                        f"{lines[-1] if lines else 'пустой вывод'}")
        if not os.path.isfile(report_path) or os.path.getsize(report_path) == 0:
            return fail("no_report",
                        "wfuzz не дал JSON-репорт (проверьте wordlist и цель)")
        try:
            with open(report_path, encoding="utf-8") as f:
                raw = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON wfuzz: {e}",
                        exit_code=2)

    if not isinstance(raw, list):
        return fail("bad_report", "ожидался список результатов wfuzz",
                    exit_code=2)

    results = []
    for entry in raw:
        if not isinstance(entry, dict):
            continue
        results.append({
            "url": str(entry.get("url") or ""),
            "payload": str(entry.get("payload") or ""),
            "code": as_int(entry.get("code")),
            "lines": as_int(entry.get("lines")),
            "words": as_int(entry.get("words")),
            "chars": as_int(entry.get("chars")),
        })

    return ok({"url": url, "results": results, "count": len(results)})


if __name__ == "__main__":
    sys.exit(main())
