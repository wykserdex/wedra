#!/usr/bin/env python3
"""dir_lister — инвентаризация файлов в директории: счёт по расширениям, размер.

permissions.filesystem: workspace — плагин обязан работать только внутри
переданного пути, наружу (../, абсолютные пути вне workspace) не выходит.
"""
import json
import os
import sys


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return 1


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"status": "error", "error": {
            "code": "bad_input", "message": str(e), "retryable": False}},
            ensure_ascii=False))
        return 2

    path = str(data.get("path") or "").strip()
    if not path:
        return fail("empty_input", "поле path пустое")

    # защита от выхода за пределы workspace: запрещаем абсолютные пути
    # и любой относительный путь, который после нормализации всё ещё
    # начинается с "..' (т.е. поднимается выше текущей директории)
    if os.path.isabs(path):
        return fail("path_escape", "абсолютные пути запрещены (filesystem: workspace)")
    norm = os.path.normpath(path)
    if norm == ".." or norm.startswith(".." + os.sep):
        return fail("path_escape", "путь пытается выйти за пределы workspace")

    if not os.path.exists(norm):
        return fail("not_found", f"путь не найден: {norm}", retryable=False)

    if not os.path.isdir(norm):
        return fail("not_a_dir", f"путь не является директорией: {norm}")

    by_ext = {}
    total_size = 0
    file_count = 0
    try:
        for entry in os.scandir(norm):
            if entry.is_file():
                file_count += 1
                size = entry.stat().st_size
                total_size += size
                ext = os.path.splitext(entry.name)[1].lower() or "(no_ext)"
                by_ext[ext] = by_ext.get(ext, 0) + 1
    except PermissionError as e:
        return fail("permission_denied", str(e), retryable=False)

    return ok({
        "path": norm,
        "file_count": file_count,
        "total_size_bytes": total_size,
        "by_extension": by_ext,
    })


if __name__ == "__main__":
    sys.exit(main())
