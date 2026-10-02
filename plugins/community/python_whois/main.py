#!/usr/bin/env python3
"""python_whois — WHOIS по домену (обёртка над библиотекой python-whois).

Вход (stdin JSON): domain, wall_timeout (опц., общий лимит, 300).

Вызов: PYTHON_WHOIS_BIN=<бинарь> <domain>, иначе
       <python> -c SNIPPET <domain>       (cwd = временная папка).

`pip install python-whois` (richardpenman/whois, 0.9.6) — библиотека: в
setup.py нет ни scripts=, ни console_scripts, CLI-скрипта у пакета нет
(скрипт `whois` в PyPI принадлежит другому пакету). Поэтому прод-путь —
паттерн C: фиксированный сниппет импортирует whois, зовёт whois.whois() и
печатает JSON в stdout. Вшитых секретов в сниппете нет. PYTHON_WHOIS_BIN
переопределяет запуск целиком — это единственный путь для контракт-тестов.

record — распознанные поля whois без сырого `text` (в нём весь ответ сервера
целиком, он не структурирован). Пустой ответ сервера — это record={} и
found=false, а не ошибка.

Выход (stdout JSON): {domain, record{...}, found}. Доменные ошибки:
empty_domain, python_whois_not_installed, timeout (retryable), no_report,
tool_failed. Платформенные (exit 2): битый JSON входа, нечитаемый JSON
сниппета/инструмента, нечисловой wall_timeout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300
NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)

SNIPPET = (
    "import json, sys\n"
    "import whois\n"
    "entry = whois.whois(sys.argv[1])\n"
    "record = {}\n"
    "for key, value in dict(entry).items():\n"
    "    if key == 'text':\n"
    "        continue\n"
    "    record[str(key)] = value\n"
    "print(json.dumps(record, default=str, ensure_ascii=False))\n"
)


try:
    sys.stdin.reconfigure(encoding="utf-8", errors="replace")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
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
    bin_env = os.environ.get("PYTHON_WHOIS_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя ищем which'ом,
        # путь — приводим к абсолютному (моки лежат рядом с main.py).
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    return [sys.executable, "-c", SNIPPET]


def normalize_domain(raw):
    value = raw.strip().lower()
    for scheme in ("http://", "https://"):
        if value.startswith(scheme):
            value = value[len(scheme):]
    value = value.split("/", 1)[0].split("?", 1)[0].strip().rstrip(".")
    if value.startswith("www."):
        value = value[4:]
    return value


def build_cmd(domain):
    cmd = resolve_bin()
    if cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd.append(domain)
    return cmd


def has_data(record):
    for key in ("domain_name", "domain", "registrar", "creation_date",
                "expiration_date", "status", "name_servers"):
        if record.get(key):
            return True
    return False


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    domain = normalize_domain(str(data.get("domain") or ""))
    if not domain:
        return fail("empty_domain", "domain пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = build_cmd(domain)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("python_whois_not_installed",
                        "python-whois не запустился: pip install python-whois "
                        "(или укажите PYTHON_WHOIS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"whois не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = (proc.stdout or "").strip()
        blob = stdout + "\n" + (proc.stderr or "")
        tail = [ln for ln in blob.strip().splitlines() if ln.strip()]
        last = tail[-1] if tail else f"exit {proc.returncode}"

        if proc.returncode != 0:
            if NOT_MODULE_RE.search(blob):
                return fail("python_whois_not_installed",
                            "пакет whois не установлен: pip install "
                            "python-whois")
            return fail("tool_failed", f"whois упал: {last}")

    if not stdout:
        return fail("no_report", "whois ничего не напечатал")

    try:
        record = json.loads(stdout)
    except Exception as e:
        return fail("bad_report", f"не разобран JSON whois: {e}", exit_code=2)

    if not isinstance(record, dict):
        return fail("bad_report", "whois вернул не объект", exit_code=2)

    return ok({"domain": domain, "record": record, "found": has_data(record)})


if __name__ == "__main__":
    sys.exit(main())
