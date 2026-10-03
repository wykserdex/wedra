#!/usr/bin/env python3
"""volatility3 — анализ дампа памяти (обёртка над CLI vol из volatility3).

Вход (stdin JSON): image (путь к дампу, обязателен), module (опц., имя плагина
vol, по умолчанию banners.Banners), volatility_config (опц., путь к JSON-конфигу
vol или сам JSON-объект), wall_timeout (опц., общий лимит, 300).

Вызов: <VOLATILITY3_BIN|vol|volatility3> -f <image> -o <tmp> -q -r json
       [-c <config>] <module>  (cwd = временная папка).

volatility3 2.x печатает результат плагина JSON-рендерером в stdout — список
объектов, где ключи это колонки плагина, а у деревьев поле __children; при
structured_output баннер и логи уходят в stderr, так что stdout — это чистый
JSON-рендер (проверено на 2.28.2). Каталог -o всегда временный: туда пишут
файловые плагины (layerwriter, configwriter, memmap, dumpfiles, …), в репозиторий
ничего попасть не может. Кэш символов держит сам vol, отдельно от -o: на Windows
это %APPDATA%\volatility3, иначе XDG_CACHE_HOME|~/.cache/volatility3 — сюда он
попадёт при любом запуске, минуя наш каталог. Дерево __children разворачиваем в
плоский список строк.

Выход (stdout JSON): {image, module, rows[{колонка: значение}], count}.
Пустой репорт (плагин не нашёл строк) — нормальный ok с rows=[].
Доменные ошибки: empty_image, missing_file, bad_module, bad_volatility_config,
volatility3_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечитаемый stdout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_MODULE = "banners.Banners"
DEFAULT_WALL = 300
MODULE_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_.]{0,63}$")


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
    bin_env = os.environ.get("VOLATILITY3_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        found = shutil.which(bin_env)
        return [found or os.path.abspath(bin_env)]
    for name in ("vol", "volatility3"):
        found = shutil.which(name)
        if found:
            return [found]
    return []


def scalar(value):
    if value is None:
        return ""
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value
    return str(value)


def flatten(nodes, acc):
    for node in nodes:
        if not isinstance(node, dict):
            continue
        acc.append({str(k): scalar(v) for k, v in node.items()
                    if k != "__children"})
        children = node.get("__children")
        if isinstance(children, list):
            flatten(children, acc)


def parse_report(text):
    stripped = text.strip()
    if not stripped:
        return None
    candidates = [stripped]
    for opener in ("[", "{"):
        at = stripped.find(opener)
        if at > 0:
            candidates.append(stripped[at:])
    for candidate in candidates:
        try:
            return json.loads(candidate)
        except ValueError:
            continue
    return None


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    image = str(data.get("image") or "").strip()
    if not image:
        return fail("empty_image", "image пуст")
    image_path = os.path.abspath(os.path.expanduser(image))
    if not os.path.isfile(image_path):
        return fail("missing_file", f"файл дампа не найден: {image}")

    module = str(data.get("module") or "").strip() or DEFAULT_MODULE
    if not MODULE_RE.match(module):
        return fail("bad_module", "module — имя плагина vol "
                                   "(например windows.pslist или banners.Banners), "
                                   f"получено: {module}")

    config_path = ""
    config_text = ""
    config_arg = str(data.get("volatility_config") or "").strip()
    if config_arg:
        if os.path.isfile(config_arg):
            config_path = os.path.abspath(config_arg)
        else:
            try:
                parsed = json.loads(config_arg)
            except ValueError as e:
                return fail("bad_volatility_config",
                            "volatility_config — не существующий файл и не "
                            f"валидный JSON: {e}")
            if not isinstance(parsed, dict):
                return fail("bad_volatility_config",
                            "volatility_config JSON должен быть объектом")
            config_text = config_arg

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_cmd = resolve_bin()
    if not bin_cmd:
        return fail("volatility3_not_installed",
                    "volatility3 не найден: pip install volatility3 "
                    "(CLI-скрипт vol) или укажите VOLATILITY3_BIN")
    if len(bin_cmd) == 1 and bin_cmd[0].lower().endswith(".py"):
        bin_cmd = [sys.executable] + bin_cmd

    with tempfile.TemporaryDirectory() as td:
        cmd = list(bin_cmd) + ["-f", image_path, "-o", td, "-q",
                               "-r", "json"]
        if config_path:
            cmd += ["-c", config_path]
        elif config_text:
            written = os.path.join(td, "volatility_config.json")
            with open(written, "w", encoding="utf-8") as f:
                f.write(config_text)
            cmd += ["-c", written]
        cmd.append(module)

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  errors="replace", cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("volatility3_not_installed",
                        "volatility3 не найден: pip install volatility3 "
                        "(CLI-скрипт vol) или укажите VOLATILITY3_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"volatility3 не уложился в {wall:.0f}s: дамп большой — "
                        "увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"vol вернул код {proc.returncode}: {last}")
        text = proc.stdout or ""
        if not text.strip():
            return fail("no_report",
                        "volatility3 не напечатал репорт (--renderer json)")
        report = parse_report(text)

    if report is None:
        return fail("bad_report", "не разобран JSON-вывод volatility3",
                    exit_code=2)
    if isinstance(report, dict):
        report = [report]
    if not isinstance(report, list):
        return fail("bad_report", "неожиданный формат репорта volatility3",
                    exit_code=2)

    rows = []
    flatten(report, rows)
    return ok({"image": image, "module": module, "rows": rows,
               "count": len(rows)})


if __name__ == "__main__":
    sys.exit(main())