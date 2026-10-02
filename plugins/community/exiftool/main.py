#!/usr/bin/env python3
"""exiftool — чтение метаданных файла (EXIF/IPTC/XMP, GPS, устройство).

Вход (stdin JSON): file (обязательный путь к файлу), wall_timeout (опц., 60).

Запуск, cwd = временная папка, stdin закрыт:
  1) <EXIFTOOL_BIN | exiftool из PATH> -j -q <file>  → JSON-массив в stdout;
  2) системного бинаря нет → паттерн C (спека §8a):
     <python> -c <SNIPPET> <file>, где сниппет импортирует pip-пакет
     pyexiftool (находит перл-бинарь сам, в т.ч. в каталоге установки под
     Windows) и печатает тот же JSON. Отдельным CLI-донором pyexiftool не
     является: консольного скрипта у пакета нет.

Выход (stdout JSON): {file, tags{}, count}. tags — первый объект массива
exiftool без SourceFile, значения приведены к строкам (списки — через
json.dumps). Пустой массив «[]» — «тегов нет», это ok с count=0.

Доменные ошибки: empty_file, missing_file, exiftool_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2):
битый JSON входа, нечитаемый репорт exiftool.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 60
MAX_TAGS = 400

SNIPPET = (
    "import json,sys,exiftool;"
    "e=exiftool.ExifTool();"
    "print(json.dumps(e.execute_json(sys.argv[1]),ensure_ascii=False,default=str));"
    "e.close()"
)


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


def build_cmd(target):
    """Командная строка чтения метаданных: системный exiftool, иначе сниппет."""
    bin_env = os.environ.get("EXIFTOOL_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя ищем which'ем,
        # путь — приводим к абсолютному
        if "/" in bin_env or "\\" in bin_env:
            base = os.path.abspath(bin_env)
        else:
            base = shutil.which(bin_env) or os.path.abspath(bin_env)
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        if base.lower().endswith(".py"):
            base = [sys.executable, base]
        else:
            base = [base]
        return base + ["-j", "-q", target]
    found = shutil.which("exiftool")
    if found:
        return [found, "-j", "-q", target]
    return [sys.executable, "-c", SNIPPET, target]


def tag_text(value):
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

    raw = data.get("file")
    if raw is not None and not isinstance(raw, str):
        return fail("bad_file", f"file должен быть строкой, пришло {type(raw).__name__}",
                    exit_code=2)
    path = (raw or "").strip()
    if not path:
        return fail("empty_file", "file пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом", exit_code=2)

    if not os.path.isfile(path):
        return fail("missing_file",
                    f"файл не найден: {path} (cwd={os.getcwd()})")

    cmd = build_cmd(path)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, cwd=td,
                                  stdin=subprocess.DEVNULL, timeout=wall)
        except FileNotFoundError:
            return fail("exiftool_not_installed",
                        "exiftool не найден: поставьте системный ExifTool "
                        "(exiftool.org) или pip install pyexiftool "
                        "(или укажите EXIFTOOL_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"exiftool не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stderr = proc.stderr or ""
        if proc.returncode != 0:
            if "ModuleNotFoundError" in stderr or "No module named" in stderr:
                return fail("exiftool_not_installed",
                            "нет ни exiftool в PATH, ни pyexiftool: "
                            "поставьте ExifTool (exiftool.org) или "
                            "pip install pyexiftool")
            tail = stderr.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"exiftool вернул код {proc.returncode}: {last}")

        out = (proc.stdout or "").strip()
        if not out:
            return fail("no_report", "exiftool не выдал JSON в stdout")
        try:
            report = json.loads(out)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON exiftool: {e}",
                        exit_code=2)

    if isinstance(report, dict):
        entries = [report]
    elif isinstance(report, list):
        entries = [x for x in report if isinstance(x, dict)]
    else:
        return fail("bad_report",
                    "неожиданный формат репорта exiftool: не массив объектов",
                    exit_code=2)

    tags = {}
    for key, value in (entries[0] if entries else {}).items():
        if key == "SourceFile":
            continue
        tags[str(key)] = tag_text(value)
        if len(tags) >= MAX_TAGS:
            break

    return ok({"file": path, "tags": tags, "count": len(tags)})


if __name__ == "__main__":
    sys.exit(main())