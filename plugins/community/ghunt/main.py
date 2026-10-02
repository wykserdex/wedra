#!/usr/bin/env python3
"""ghunt — OSINT по Google-аккаунтам через CLI GHunt (mxrch/GHunt).

Вход (stdin JSON): mode (email|username, обязательный), target (обязательный),
wall_timeout (опц., 300).

Запуск, cwd = временная папка, stdin закрыт:
  <GHUNT_BIN | ghunt> <mode> <target> --json <файл во временной папке>
GHunt сам читает токен/сессию из env GHUNT_TOKEN — плагин переменную только
пробрасывает в дочерний процесс и не требует её наличия, поэтому
happy-path полностью мокается.

О подкомандах: по документации GHunt 2.x из CLI-экспорта с --json есть
login, email, gaia, drive, geolocate (плюс spiderdal без --json). Подкоманды
username у апстрима НЕТ. mode пробрасывается в командную строку как есть,
ничего не подменяем и не изобретаем: mode=username на текущем GHunt
закончится tool_failed. mode=email — рабочий сценарий.

Выход (stdout JSON): {target, records[], count}. records — плоский список
{field, value}: верхний уровень JSON-выгрузки GHunt (dict → по ключам,
list/scalar → одним элементом). Форма выгрузки апстримом не зафиксирован,
поэтому разбор намеренно консервативный. Пустая выгрузка = «ничего не
нашлось» (ok, count=0).

Доменные ошибки: bad_mode, empty_target, ghunt_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, нечитаемая выгрузка.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

MODES = ("email", "username")
DEFAULT_WALL = 300
MAX_RECORDS = 500


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
    """Путь к CLI GHunt: env, потом ghunt из PATH."""
    bin_env = os.environ.get("GHUNT_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя ищем which'ем,
        # путь — приводим к абсолютному
        if "/" in bin_env or "\\" in bin_env:
            return os.path.abspath(bin_env)
        return shutil.which(bin_env) or os.path.abspath(bin_env)
    return shutil.which("ghunt")


def value_text(value):
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False, default=str)


def records_from(report):
    """Плоский разбор выгрузки GHunt: [{field, value}] — форма не зафиксирована."""
    out = []
    if isinstance(report, dict):
        for key, value in report.items():
            out.append({"field": str(key), "value": value_text(value)})
    elif isinstance(report, list):
        for item in report:
            out.append({"field": "", "value": value_text(item)})
    else:
        out.append({"field": "", "value": value_text(report)})
    return out[:MAX_RECORDS]


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    raw_mode = data.get("mode")
    if raw_mode is not None and not isinstance(raw_mode, str):
        return fail("bad_mode",
                    f"mode должен быть строкой, пришло {type(raw_mode).__name__}")
    mode = (raw_mode or "").strip().lower()
    if not mode:
        return fail("bad_mode", "mode пуст")
    if mode not in MODES:
        return fail("bad_mode",
                    f"mode должен быть одним из {', '.join(MODES)}")

    target = str(data.get("target") or "").strip()
    if not target:
        return fail("empty_target", "target пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом", exit_code=2)

    base = resolve_bin()
    if base is None:
        return fail("ghunt_not_installed",
                    "ghunt не найден в PATH: pipx install ghunt "
                    "(или укажите GHUNT_BIN)")

    # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
    cmd = [sys.executable, base] if base.lower().endswith(".py") else [base]

    token = os.environ.get("GHUNT_TOKEN", "")
    env = os.environ.copy()
    if token:
        env["GHUNT_TOKEN"] = token

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, f"ghunt_{mode}.json")
        cmd += [mode, target, "--json", report_path]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, cwd=td,
                                  stdin=subprocess.DEVNULL, timeout=wall,
                                  env=env)
        except FileNotFoundError:
            return fail("ghunt_not_installed",
                        "ghunt не найден: pipx install ghunt "
                        "(или укажите GHUNT_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"ghunt не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"ghunt вернул код {proc.returncode}: {last}")

        if not os.path.isfile(report_path):
            return fail("no_report", "ghunt не записал JSON-выгрузку")
        try:
            with open(report_path, encoding="utf-8") as f:
                raw = f.read().strip()
        except Exception as e:
            return fail("bad_report", f"не прочитана выгрузка ghunt: {e}",
                        exit_code=2)
        if not raw:
            return fail("no_report", "выгрузка ghunt пуста")
        try:
            report = json.loads(raw)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON ghunt: {e}",
                        exit_code=2)

    return ok({"target": target, "records": records_from(report),
               "count": len(records_from(report))})


if __name__ == "__main__":
    sys.exit(main())