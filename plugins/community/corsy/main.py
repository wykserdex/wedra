#!/usr/bin/env python3
"""corsy — поиск CORS-мисконфигураций по одной явно заданной цели (CLI Corsy).

Аудит без разведки: сканируется ровно один url, который дал пользователь (файлов
со списком целей, краулинга и subdomain-разведки нет). Только GET-пробы с
Origin-заголовком — Corsy ничего не пишет на цель, состояние не меняет.

Вход (stdin JSON): url, origins (опц.), wall_timeout (опц., общий лимит, 300).

Вызов: <CORSY_BIN|corsy.py> -u <url> -o report.json -q  (cwd = временная
папка). Флаг -i у Corsy — это входной ФАЙЛ со списком url/subdomain, а не
цель, поэтому цель уходит через -u.

Corsy v1.0-beta не умеет принимать список Origin: core/tests.active_tests
перебирает свой фиксированный набор (example.com, <root>.example.com, d3v<root>,
null, <root>_.example.com, <root>%60.example.com, http://<root> и — только для
многоуровневого домена — <root> с первой точкой, заменённой на «x»).
Поэтому поле origins только проверяется на тип и в команду не попадает — в
finding попадает тот Origin, который сервер реально отразил в ACAO.

Машинный отчёт `-o` — JSON-словарь {url: {class, description, severity,
exploitation, "acao header", "acac header"}}; Corsy пишет его только когда
нашёл хоть одну мисконфигурацию, поэтому отсутствие файла при коде 0 — это
«находок нет», а не ошибка.

Выход (stdout JSON): {url, findings[{origin, url, vulnerable}], count}.
Находок нет — нормальный результат (findings=[], count=0). Доменные ошибки:
empty_url, corsy_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, некорректные поля входа, нечитаемый
JSON-отчёт.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

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

    origins = data.get("origins")
    if origins is not None:
        if not isinstance(origins, list):
            return fail("bad_origins", "origins обязан быть массивом",
                        exit_code=2)
        for origin in origins:
            if not isinstance(origin, str):
                return fail("bad_origins",
                            "origins обязан быть массивом строк", exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin("CORSY_BIN", "corsy.py")
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        cmd += ["-u", url, "-o", report_path, "-q"]
        try:
            # stdin в DEVNULL: при не-tty Corsy читает его целиком, а читать
            # нечего — блокировать нечему.
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("corsy_not_installed",
                        "corsy не найден: поставьте из исходников "
                        "(git clone https://github.com/s0md3v/Corsy && "
                        "pip install -r requirements.txt) и укажите путь "
                        "к corsy.py в CORSY_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"corsy не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"corsy упал с кодом {proc.returncode}: {last}")
        if not os.path.exists(report_path):
            # Corsy пишет -o только когда что-то нашёл.
            return ok({"url": url, "findings": [], "count": 0})
        if os.path.getsize(report_path) == 0:
            return fail("no_report", f"corsy дал пустой отчёт ({REPORT_NAME})")
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON corsy: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат отчёта corsy",
                    exit_code=2)

    findings = []
    for target, info in report.items():
        if not isinstance(info, dict):
            return fail("bad_report", "запись отчёта corsy не объект",
                        exit_code=2)
        findings.append({
            "origin": str(info.get("acao header") or ""),
            "url": str(target),
            "vulnerable": True,
        })
    findings.sort(key=lambda item: (item["url"], item["origin"]))

    return ok({"url": url, "findings": findings, "count": len(findings)})


if __name__ == "__main__":
    sys.exit(main())