#!/usr/bin/env python3
"""pocsuite3 — проверка URL на PoC из набора PoC Suite3 (обёртка над CLI pocsuite).

Вход (stdin JSON): url (единственная цель), keyword (опц., -k), timeout (опц.,
--timeout, 10), wall_timeout (опц., общий лимит, 300).

Вызов: <POCSUITE3_BIN|pocsuite> -u <url> --verify -o <report.jsonl>
       [-k KEYWORD] [--timeout N]  (cwd = временная папка).

Точка входа. У pocsuite3 2.1.0 (pypi wheel pocsuite3-2.1.0.dist-info/
entry_points.txt) ровно два консольных скрипта: `pocsuite` (pocsuite3.cli:main)
и `poc-console` (интерактивная консоль). Скрипта `poc` у пакета НЕТ — такого
entry_point не было и в 1.9.8, и в 1.2.10; модуля `__main__` тоже нет, поэтому
`python -m pocsuite3` не работает. resolve_bin ищет `pocsuite`, затем `poc`, и
никогда не подставляет `python -m`.

Флаги сверены с pocsuite3/lib/parse/cmd.py 2.1.0 (и с живым `pocsuite --help`
2.1.0): -u/--url (nargs='+'), --verify (режим по умолчанию), -k (фильтр PoC по
ключу), --timeout (float, default 10), --threads. Флага `--format` у pocsuite3 НЕТ:
машинный вывод один — -o/--output, «Output file to write (JSON Lines format)»:
плагин file_record (pocsuite3/plugins/file_record.py) пишет в этот файл по строке на
УСПЕШНУЮ находку: {"target", "poc_name", "result", "created_time"}. Разбор
консервативный: непустой мусор → bad_report (exit 2), пустой файл → ok с
vulns=[] («ничего не нашлось»).

Флага `--batch` НЕ передаём намеренно: в 2.1.0 он объявлен БЕЗ action=store_true,
то есть требует значения (`--batch BATCH`, optiondict.py: 'batch': 'string'), и
голый `--batch` роняет argparse — «error: argument --batch: expected one
argument», файл -o не создаётся. При этом сам conf.batch НИГДЕ не читается
(только _reset_option ставит False), так что неинтерактивность он всё равно не
даёт: input() в пути --verify есть только для режима --shell. Отдельно: cli.main()
глотает SystemExit, поэтому на любой ошибке разбора командной строки pocsuite
выходит с кодом 0 — ориентируемся на отсутствие файла -o, а не на код возврата.

Выход (stdout JSON): {url, vulns[{poc,target}], count}. Доменные ошибки:
empty_url, bad_url, bad_timeout, pocsuite3_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, нечисловой
wall_timeout, нечитаемый отчёт (bad_report).
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

DEFAULT_TIMEOUT = 10
DEFAULT_WALL = 300
REPORT_NAME = "pocsuite_report.jsonl"


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def number_or_none(value):
    if value is None or value == "" or isinstance(value, bool):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def resolve_bin():
    bin_env = os.environ.get("POCSUITE3_BIN", "").strip()
    if bin_env:
        if "/" not in bin_env and "\\" not in bin_env:
            return shutil.which(bin_env) or os.path.abspath(bin_env)
        return os.path.abspath(bin_env)
    for name in ("pocsuite", "poc"):
        found = shutil.which(name)
        if found:
            return found
    return None


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

    keyword = str(data.get("keyword") or "").strip()

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

    binary = resolve_bin()
    if binary is None:
        return fail("pocsuite3_not_installed",
                    "pocsuite не найден в PATH: pip install pocsuite3 "
                    "(консольный скрипт pocsuite) — или укажите "
                    "POCSUITE3_BIN; у пакета нет ни скрипта `poc`, "
                    "ни модуля для python -m")
    cmd = [binary]
    if binary.lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable] + cmd
    cmd += ["-u", url, "--verify",
            "--timeout",
            str(int(timeout if timeout is not None else DEFAULT_TIMEOUT))]
    if keyword:
        cmd += ["-k", keyword]

    with tempfile.TemporaryDirectory() as td:
        cmd += ["-o", os.path.join(td, REPORT_NAME)]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("pocsuite3_not_installed",
                        "pocsuite не найден: pip install pocsuite3 "
                        "(или укажите POCSUITE3_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"pocsuite не уложился в {wall:.0f}s: уменьшите "
                        "keyword или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        report_path = os.path.join(td, REPORT_NAME)
        if not os.path.isfile(report_path):
            combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
            tail = combined.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"pocsuite упал (exit {proc.returncode}): {last}")
            return fail("no_report",
                        f"pocsuite не дал отчёт -o: {last}")
        try:
            with open(report_path, encoding="utf-8") as f:
                raw_lines = f.read().splitlines()
        except Exception as e:
            return fail("bad_report", f"не прочитан отчёт pocsuite: {e}",
                        exit_code=2)

    vulns = []
    seen = set()
    for raw in raw_lines:
        line = raw.strip()
        if not line:
            continue
        try:
            record = json.loads(line)
        except ValueError as e:
            return fail("bad_report",
                        f"строка отчёта pocsuite не JSON ({e}): {line[:120]}",
                        exit_code=2)
        if not isinstance(record, dict):
            return fail("bad_report",
                        "строка отчёта pocsuite не объект: " + line[:120],
                        exit_code=2)
        poc = str(record.get("poc_name") or "").strip()
        if not poc:
            continue
        key = (poc, str(record.get("target") or url).strip())
        if key in seen:
            continue
        seen.add(key)
        vulns.append({"poc": poc, "target": key[1]})

    return ok({"url": url, "vulns": vulns, "count": len(vulns)})


if __name__ == "__main__":
    sys.exit(main())