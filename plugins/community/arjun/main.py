#!/usr/bin/env python3
"""arjun — фаззинг скрытых GET-параметров (обёртка над CLI arjun).

Вход (stdin JSON): url (единственная цель), stable (опц., bool), chunks (опц., -c),
timeout (опц., -T, 15), wall_timeout (опц., общий лимит, 300).

Вызов: <ARJUN_BIN|arjun|python3 -m arjun> -u <url> -oJ arjun_report.json
       [-c N] [-T N] [--stable]  (cwd = временная папка).

Флаги сверены с arjun/__main__.py (s0md3v/Arjun 2.2.7): -u/--url — цель,
-o/-oJ <file> — JSON-репорт (длинной формы --output-json у arjun НЕТ, только
-o и -oJ), -c — размер чанка имён, -T — таймаут запроса, --stable — 1 поток и
случайная задержка между запросами. Свою разведку не запускаем: цель только та,
что задал оператор (--passive/-i не используем намеренно).

Отчёт: arjun пишет -oJ только когда нашёл хоть один параметр, JSON вида
{"<url>": {"params": [...], "method": "GET", "headers": {...}}}. Если параметров
не найдено, файл не создаётся, а в stdout идёт "No parameters were
discovered." — это нормальный пустой результат (ok, parameters=[]), а не ошибка.
Отчёт не появился по другой причине (нет ни файла, ни этой строки) — no_report;
"Skipped ... due to errors" / ненулевой код возврата — tool_failed.

Выход (stdout JSON): {url, parameters[], count}. Доменные ошибки: empty_url,
bad_url, bad_stable, bad_chunks, bad_timeout, arjun_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
нечисловой wall_timeout, нечитаемый репорт (bad_report).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
except Exception:
    pass

DEFAULT_TIMEOUT = 15
DEFAULT_WALL = 300
REPORT_NAME = "arjun_report.json"


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def as_bool(value, default):
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        low = value.strip().lower()
        if low in ("1", "true", "yes", "on"):
            return True
        if low in ("", "0", "false", "no", "off"):
            return False
        return default
    return default


def number_or_none(value):
    if value is None or value == "":
        return None
    if isinstance(value, bool):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def resolve_bin():
    bin_env = os.environ.get("ARJUN_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем which'ем,
        # путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            return [shutil.which(bin_env) or os.path.abspath(bin_env)]
        return [os.path.abspath(bin_env)]
    found = shutil.which("arjun")
    if found:
        return [found]
    return [sys.executable, "-m", "arjun"]


def pick_params(report, url):
    if url in report:
        entry = report[url]
    elif len(report) == 1:
        entry = list(report.values())[0]
    else:
        entry = None
        for value in report.values():
            if isinstance(value, dict) and value.get("params"):
                entry = value
                break
        if entry is None:
            entry = next(iter(report.values()), None)
    if not isinstance(entry, dict):
        return []
    params = entry.get("params")
    if not isinstance(params, list):
        return []
    out = []
    for item in params:
        text = str(item).strip()
        if text and text not in out:
            out.append(text)
    return sorted(out)


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
    if "://" not in url:
        url = "https://" + url
    if not url.startswith(("http://", "https://")):
        return fail("bad_url", f"схема не http(s): {url}")

    if data.get("stable") is not None and not isinstance(data.get("stable"),
                                                          (bool, str, int,
                                                           float)):
        return fail("bad_stable", "stable должен быть boolean")
    stable = as_bool(data.get("stable"), False)

    chunks = number_or_none(data.get("chunks"))
    if chunks is None and data.get("chunks") not in (None, ""):
        return fail("bad_chunks", "chunks должен быть числом")
    if chunks is not None and chunks < 1:
        return fail("bad_chunks", "chunks должен быть >= 1")

    timeout = number_or_none(data.get("timeout"))
    if timeout is None and data.get("timeout") not in (None, ""):
        return fail("bad_timeout", "timeout должен быть числом")
    if timeout is not None and timeout < 1:
        return fail("bad_timeout", "timeout должен быть >= 1")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd += ["-u", url, "-oJ", report_path]
        if chunks is not None:
            cmd += ["-c", str(int(chunks))]
        cmd += ["-T", str(int(timeout if timeout is not None
                               else DEFAULT_TIMEOUT))]
        if stable:
            cmd.append("--stable")

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("arjun_not_installed",
                        "arjun не найден: pip install arjun "
                        "(или укажите ARJUN_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"arjun не уложился в {wall:.0f}s: уменьшите chunks "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if os.path.isfile(report_path):
            try:
                with open(report_path, encoding="utf-8") as f:
                    report = json.load(f)
            except Exception as e:
                return fail("bad_report", f"не прочитан JSON arjun: {e}",
                            exit_code=2)
        else:
            report = None

    if report is None:
        tail = (combined or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        if proc.returncode != 0:
            return fail("tool_failed",
                        f"arjun упал (exit {proc.returncode}): {last}")
        if "skipped" in combined.lower():
            return fail("tool_failed", f"arjun пропустил цель: {last}")
        if "no parameters were discovered" in combined.lower():
            return ok({"url": url, "parameters": [], "count": 0})
        return fail("no_report", f"arjun не дал JSON-репорт: {last}")

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат репорта arjun",
                    exit_code=2)

    parameters = pick_params(report, url)
    return ok({"url": url, "parameters": parameters, "count": len(parameters)})


if __name__ == "__main__":
    sys.exit(main())