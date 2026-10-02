#!/usr/bin/env python3
"""spiderfoot — неинтерактивный OSINT-скан цели через CLI SpiderFoot (sf.py).

Вход (stdin JSON): target (обязательный), module (опц., список модулей через
запятую), wall_timeout (опц., 600).

Запуск, cwd = временная папка, stdin закрыт (интерактив не нужен):
  <SPIDERFOOT_BIN | spiderfoot | sf.py | sf> -s <target> -o json -q
      [-m <module>]
Отчёт НЕ кладётся в temp-CWD: sf.py в режиме скана печатает результат в
stdout через sfp__stor_stdout (при -o json — JSON-массив, печатается по
событиям: сначала "[", затем элементы через запятую, в конце "]"), а БД
сканера лежит в ~/.spiderfoot/spiderfoot.db. Поэтому разбираем stdout:
берём срез от первого "[" до последнего "]". Если модуль не задан, sf.py
включает ВСЕ установленные модули — скан будет долгим, задавайте module.

Выход (stdout JSON): {target, events[], count}. events — нормализованные
события {type, data, module, source} (значения — строки; поле type у sf.py
несёт человекочитаемое описание типа). Пустой массив = событий нет (ok,
count=0).

Доменные ошибки: empty_target, spiderfoot_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, нечитаемый репорт, module/wall_timeout неверного типа.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 600
MAX_EVENTS = 500


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
    """Путь к CLI SpiderFoot: env, потом spiderfoot/sf.py/sf из PATH."""
    bin_env = os.environ.get("SPIDERFOOT_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя ищем which'ем,
        # путь — приводим к абсолютному
        if "/" in bin_env or "\\" in bin_env:
            return os.path.abspath(bin_env)
        return shutil.which(bin_env) or os.path.abspath(bin_env)
    for name in ("spiderfoot", "sf.py", "sf"):
        found = shutil.which(name)
        if found:
            return found
    return None


def build_cmd(target, module):
    base = resolve_bin()
    if base is None:
        return None
    cmd = [sys.executable, base] if base.lower().endswith(".py") else [base]
    cmd += ["-s", target, "-o", "json", "-q"]
    if module:
        cmd += ["-m", module]
    return cmd


def event_text(value):
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False, default=str)


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

    module = data.get("module")
    if module is not None and not isinstance(module, str):
        return fail("bad_module",
                    f"module должен быть строкой, пришло {type(module).__name__}",
                    exit_code=2)
    module = module.strip() if isinstance(module, str) else ""

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом", exit_code=2)

    cmd = build_cmd(target, module)
    if cmd is None:
        return fail("spiderfoot_not_installed",
                    "spiderfoot не найден в PATH: pip install spiderfoot "
                    "(или укажите SPIDERFOOT_BIN путём к sf.py)")

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, cwd=td,
                                  stdin=subprocess.DEVNULL, timeout=wall)
        except FileNotFoundError:
            return fail("spiderfoot_not_installed",
                        "spiderfoot не найден: pip install spiderfoot "
                        "(или укажите SPIDERFOOT_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"spiderfoot не уложился в {wall:.0f}s: сузьте module "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"spiderfoot вернул код {proc.returncode}: {last}")

        out = (proc.stdout or "").strip()
        start = out.find("[")
        end = out.rfind("]")
        if start < 0 or end <= start:
            return fail("no_report",
                        "spiderfoot не напечатал JSON-массив событий в stdout")
        try:
            report = json.loads(out[start:end + 1])
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON spiderfoot: {e}",
                        exit_code=2)

    if isinstance(report, dict):
        entries = [report]
    elif isinstance(report, list):
        entries = report
    else:
        return fail("bad_report",
                    "неожиданный формат репорта spiderfoot", exit_code=2)

    events = []
    for item in entries:
        if not isinstance(item, dict):
            continue
        events.append({
            "type": event_text(item.get("type") or item.get("eventType")),
            "data": event_text(item.get("data")),
            "module": event_text(item.get("module")),
            "source": event_text(item.get("source")),
        })
        if len(events) >= MAX_EVENTS:
            break

    return ok({"target": target, "events": events, "count": len(events)})


if __name__ == "__main__":
    sys.exit(main())