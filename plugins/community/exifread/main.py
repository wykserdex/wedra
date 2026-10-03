#!/usr/bin/env python3
"""exifread — EXIF-метаданные изображения (обёртка над библиотекой exifread).

У донора (lexiflex/exifread) CLI с машинным выводом нет: с 3.1.0 есть exifread.cli
(console_scripts `EXIF.py`, плюс `python -m exifread`), но он печатает через
logger человекочитаемые строки `поле (ТИП): значение`, а не JSON. Поэтому
работаем паттерном C: продакшн-путь — `python -c <SNIPPET> <файл>`, где
сниппет импортирует exifread и печатает JSON в stdout. Отчёт — stdout
дочернего процесса. main.py остаётся stdlib-only и сам exifread не импортирует.

Вход (stdin JSON): file (путь к изображению), wall_timeout (опц., 60).

Вызов: если задан EXIFREAD_BIN — <EXIFREAD_BIN> <файл>, иначе
       [python, -c, SNIPPET, <файл>]  (cwd = временная папка).

Сниппет сверен с exifread/__init__.py 3.5.1: process_file(fh, stop_tag='UNDEF',
details=True, strict=False, debug=False, truncate_tags=True, auto_seek=True,
extract_thumbnail=True, builtin_types=False) -> Dict[str, Any].
details=False — без разбора MakerNote, extract_thumbnail=False — без выгрузки
JPEG/TIFF-превью (они в hdr.tags кладутся сырыми bytes, а не IfdTag).
Значение берётся из IfdTag.printable (при его отсутствии — str(tag)), всё
приводится к строкам: JSON без bytes и вложенных объектов.
Валидный, но пустой отчёт — это ok с пустым tags; «не изображение» даёт пустой
dict от exifread. Доменные ошибки: empty_file, missing_file, exifread_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, нечисловой wall_timeout, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 60
MAX_TAGS = 500
MAX_TAG_VALUE = 2000

SNIPPET = (
    "import json,sys,exifread;"
    "fh=open(sys.argv[1],'rb');"
    "tags=exifread.process_file(fh, details=False, extract_thumbnail=False);"
    "fh.close();"
    "print(json.dumps({'file':sys.argv[1],"
    "'tags':{str(k):str(getattr(v,'printable',None) or v) "
    "for k,v in tags.items()},"
    "'count':len(tags)}))"
)

NOT_MODULE_RE = re.compile(
    r"No module named|can't open file|No such file or directory",
    re.IGNORECASE)


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_cmd(path):
    bin_env = os.environ.get("EXIFREAD_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
            cmd = [sys.executable] + cmd
    else:
        cmd = [sys.executable, "-c", SNIPPET]
    return cmd + [path]


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8", errors="replace")
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    raw = data.get("file")
    if raw is None or (isinstance(raw, str) and not raw.strip()):
        return fail("empty_file", "поле file пустое")
    if not isinstance(raw, str):
        return fail("bad_file", f"file должен быть строкой, "
                    f"пришло {type(raw).__name__}", exit_code=2)

    path = raw.strip()
    if not os.path.isabs(path):
        candidate = os.path.join(os.getcwd(), path)
        if not os.path.isfile(candidate):
            return fail("missing_file",
                        f"файл не найден: {path} (cwd={os.getcwd()})")
        path = candidate
    elif not os.path.isfile(path):
        return fail("missing_file", f"файл не найден: {path}")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_cmd(path)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("exifread_not_installed",
                        "интерпретатор/скрипт не найден: pip install exifread "
                        "(или укажите EXIFREAD_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"exifread не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    blob = (proc.stdout or "") + "\n" + (proc.stderr or "")

    if proc.returncode != 0:
        if NOT_MODULE_RE.search(blob):
            return fail("exifread_not_installed",
                        "exifread не установлен: pip install exifread "
                        "(или укажите EXIFREAD_BIN)")
        tail = blob.strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"чтение EXIF упало (exit {proc.returncode}): {last}")

    stdout = (proc.stdout or "").strip()
    if not stdout:
        return fail("no_report", "сниппет exifread не напечатал отчёт")

    try:
        report = json.loads(stdout)
    except Exception as e:
        return fail("bad_report", f"не разобран JSON exifread: {e}",
                    exit_code=2)
    if not isinstance(report, dict):
        return fail("bad_report", "ожидался JSON-объект от exifread",
                    exit_code=2)

    raw_tags = report.get("tags")
    if raw_tags is None:
        raw_tags = {}
    if not isinstance(raw_tags, dict):
        return fail("bad_report", "поле tags в отчёте — не объект",
                    exit_code=2)

    tags = {}
    for name, value in list(raw_tags.items())[:MAX_TAGS]:
        text = "" if value is None else str(value)
        tags[str(name)] = text[:MAX_TAG_VALUE]

    return ok({"file": str(report.get("file") or path), "tags": tags,
               "count": len(tags)})


if __name__ == "__main__":
    sys.exit(main())
