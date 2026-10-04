#!/usr/bin/env python3
"""dir_lister — инвентаризация файлов в разрешённой директории: счёт/размер.

Runner запускает плагин с cwd=m.Dir; относительный путь остаётся под этим
корнем. Проверка realpath также закрывает symlink, указывающий наружу.
"""
import json
import os
import sys

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


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

    # Runner задаёт cwd=m.Dir. Оставляем лексические проверки и дополнительно
    # проверяем реальный путь: normpath сам по себе не разрешает symlink.
    if os.path.isabs(path) or path.startswith(("/", "\\")):
        return fail("path_escape", "абсолютные пути запрещены (filesystem: workspace)")
    norm = os.path.normpath(path)
    if norm == ".." or norm.startswith(".." + os.sep):
        return fail("path_escape", "путь пытается выйти за пределы workspace")

    allowed_root = os.path.realpath(os.getcwd())
    resolved = os.path.realpath(os.path.join(allowed_root, norm))
    try:
        within_root = os.path.commonpath((allowed_root, resolved)) == allowed_root
    except ValueError:
        within_root = False
    if not within_root:
        return fail("path_escape", "symlink выводит путь за пределы разрешённого корня")

    if not os.path.exists(resolved):
        return fail("not_found", f"путь не найден: {norm}", retryable=False)
    if not os.path.isdir(resolved):
        return fail("not_a_dir", f"путь не является директорией: {norm}")

    by_ext = {}
    total_size = 0
    file_count = 0
    try:
        for entry in os.scandir(resolved):
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
