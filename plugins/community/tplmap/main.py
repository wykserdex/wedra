#!/usr/bin/env python3
"""tplmap — поиск SSTI по одной явно заданной цели (обёртка над CLI tplmap).

Аудит без эксплуатации: передаются только -u (и, по желанию, -e), флагов
--os-cmd/--os-shell/--upload/--download/--bind-shell/--reverse-shell/--tpl-code
в команде нет. Разведки и краулинга нет — цель ровно та, что дал пользователь
(метку точки инъекции можно оставить в самом url: tplmap по умолчанию ищет
инъекции в параметрах GET/POST/заголовков).

Вход (stdin JSON): url, engine (опц.), wall_timeout (опц., общий лимит, 300).

Вызов: <TPLMAP_BIN|tplmap.py> -u <url> [-e <engine>]  (cwd = временная папка).

Отчёта-файла у tplmap нет (флагов --pipe/--report-json тоже нет) — разбираем
текстовый stdout. Формат (utils/loggers.py, префиксы [+] / [-] / [!]):
  [+] Tplmap 0.5
  [+] Testing if GET parameter 'name' is injectable
  [+] Jinja2 plugin has confirmed injection with tag '{{*}}'
  [+] Tplmap identified the following injection point:
        GET parameter: name
        Engine: Jinja2
        Injection: {{*}}
        Context: text
  [!][core.checks] Tested parameters appear to be not injectable.

Выход (stdout JSON): {url, vulnerable, engine, payloads[]}. payloads — теги,
которые движок подтвердил (строки «has confirmed injection with tag '...'»
и строка «Injection: ...» из сводки). Не нашлось — нормальный результат
(vulnerable=false, engine="", payloads=[]). Доменные ошибки: empty_url,
tplmap_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, некорректные поля входа.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

MARKER = "identified the following injection point"
CONFIRMED_RE = re.compile(r"has confirmed injection with tag ['\"](.+?)['\"]")
ENGINE_RE = re.compile(r"^\s*Engine:\s*(.+?)\s*$", re.MULTILINE)
INJECTION_RE = re.compile(r"^\s*Injection:\s*(.+?)\s*$", re.MULTILINE)

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

    raw_engine = data.get("engine")
    if raw_engine is not None and not isinstance(raw_engine, str):
        return fail("bad_engine", "engine обязан быть строкой", exit_code=2)
    engine_arg = str(raw_engine or "").strip()

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin("TPLMAP_BIN", "tplmap.py")
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        cmd += ["-u", url]
        if engine_arg:
            cmd += ["-e", engine_arg]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("tplmap_not_installed",
                        "tplmap не найден: поставьте из исходников "
                        "(git clone https://github.com/epinna/tplmap && "
                        "pip install -r requirements.txt) и укажите путь "
                        "к tplmap.py в TPLMAP_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"tplmap не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        stdout = proc.stdout or ""
        if proc.returncode != 0:
            tail = (proc.stderr or stdout).strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"tplmap упал с кодом {proc.returncode}: {last}")
        if not stdout.strip():
            return fail("no_report",
                        "tplmap не напечатал ни строки отчёта (ожидался "
                        "блок «Tplmap identified the following injection "
                        "point» или «appear to be not injectable»)")

    vulnerable = MARKER in stdout

    engine = ""
    if vulnerable:
        match = ENGINE_RE.search(stdout)
        if match:
            engine = match.group(1).strip()

    payloads = []
    for pattern in (CONFIRMED_RE, INJECTION_RE):
        for match in pattern.findall(stdout):
            tag = match.strip()
            if tag and tag not in payloads:
                payloads.append(tag)

    return ok({"url": url, "vulnerable": vulnerable, "engine": engine,
               "payloads": payloads})


if __name__ == "__main__":
    sys.exit(main())