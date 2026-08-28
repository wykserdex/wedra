#!/usr/bin/env python3
"""dir_lister — снапшот директории: file_count + by_extension.

Community-плагин №2: написан внешним автором (тестер №1) в рамках пакета №5
как проверка контракта v0.2 — манифест БЕЗ from, разводка целиком в пайплайне.
Сценарий автора: аудит "до/после" — один плагин дважды в цепочке.
Воспроизведён в репо по его отчёту. Без зависимостей, без сети, read-only.

Помни о PROTOCOL §1: cwd этого процесса — директория плагина, поэтому
относительные пути в входе отсчитываются от plugins/dir_lister/.
"""
import json
import os
import sys


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON: {e}", exit_code=2)

    path = str(data.get("path") or "").strip()
    if not path:
        return fail("empty_path", "поле path пустое")
    if not os.path.isdir(path):
        return fail("not_found", f"директория не найдена: {path} "
                                 f"(cwd плагина: {os.getcwd()})")

    by_ext = {}
    count = 0
    for _root, _dirs, files in os.walk(path):
        for f in files:
            count += 1
            ext = os.path.splitext(f)[1].lower() or "no_ext"
            by_ext[ext] = by_ext.get(ext, 0) + 1

    print(json.dumps({"status": "ok", "output": {
        "path": os.path.abspath(path),
        "file_count": count,
        "by_extension": by_ext,
    }}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
