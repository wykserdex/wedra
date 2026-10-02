#!/usr/bin/env python3
"""msoffcrypto_tool — расшифровка паролем офисного документа (обёртка).

Вход (stdin JSON): file (путь к документу), out (опц., куда писать).

Вызов: <MSOFFCRYPTO_TOOL_BIN|msoffcrypto-tool> <abspath> <out> -p <password>
       либо, если пароля в env нет, режим проверки -t -v <abspath>
       (cwd = временная папка). Аргументы подтверждены по msoffcrypto/__main__.py:
       группа -p/--password и -t/--test взаимно исключающая и обязательная, -v —
       verbose в stderr, outfile — второй позиционный аргумент.

Пароль берётся ТОЛЬКО из env MSOFFCRYPTO_PASSWORD (никогда из входа) и в
лог/вывод не попадает. Наличия пароля плагин не требует: без него идёт
`-t -v` (exit 0 — документ зашифрован, exit 1 — не зашифрован), расшифровки не
происходит, decrypted=false и output_path пустой. `out` не задан — пишем во
свежий временный каталог ОС (его НЕ убираем: путь отдаётся наружу).

Доменные ошибки: empty_file, missing_file, msoffcrypto_tool_not_installed,
tool_failed, no_report. Платформенные (exit 2): битый JSON входа, out не
строка.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300


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
    bin_env = os.environ.get("MSOFFCRYPTO_TOOL_BIN",
                             "msoffcrypto-tool").strip()
    # subprocess поедет с cwd во временной папке: имя из PATH ищем
    # which'ем, путь — приводим к абсолютному
    if "/" not in bin_env and "\\" not in bin_env:
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)
    return bin_env


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

    path = str(data.get("file") or "").strip()
    if not path:
        return fail("empty_file", "file пуст")

    out = data.get("out")
    if out is not None and not isinstance(out, str):
        return fail("bad_out", "out обязан быть строкой", exit_code=2)
    out = (out or "").strip()

    if not os.path.exists(path):
        return fail("missing_file", f"файл не найден: {path}")
    if os.path.isdir(path):
        return fail("missing_file", f"ожидался файл, а не каталог: {path}")
    target = os.path.abspath(path)

    password = os.environ.get("MSOFFCRYPTO_PASSWORD", "")
    bin_env = resolve_bin()

    with tempfile.TemporaryDirectory() as td:
        if password:
            out_path = os.path.abspath(out) if out else os.path.join(
                tempfile.mkdtemp(prefix="wedra_msoffcrypto_"),
                "decrypted" + os.path.splitext(target)[1])
            if out:
                parent = os.path.dirname(out_path)
                if parent and not os.path.isdir(parent):
                    try:
                        os.makedirs(parent, exist_ok=True)
                    except OSError as e:
                        return fail("bad_out",
                                    f"не создать каталог для out: {e}")
            cmd = [bin_env, target, out_path, "-p", password]
        else:
            # пароля в env нет — не расшифровываем, только проверяем документ
            out_path = ""
            cmd = [bin_env, "-t", "-v", target]
        if bin_env.lower().endswith(".py"):
            # v0.29: .py-мок запускаем через интерпретатор — прямой exec
            # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
            cmd = [sys.executable] + cmd

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=DEFAULT_WALL)
        except FileNotFoundError:
            return fail("msoffcrypto_tool_not_installed",
                        "msoffcrypto-tool не найден: "
                        "pip install msoffcrypto-tool "
                        "(или укажите MSOFFCRYPTO_TOOL_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"msoffcrypto-tool не уложился в {DEFAULT_WALL:.0f}s",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        if not password:
            # -t: 0 = зашифрован, 1 = не зашифрован, >1 = ошибка
            if proc.returncode in (0, 1):
                return ok({"file": target, "decrypted": False,
                           "output_path": ""})
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            return fail("tool_failed",
                        f"msoffcrypto-tool упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")

        if proc.returncode != 0:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            return fail("tool_failed",
                        f"msoffcrypto-tool упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")

        if not os.path.isfile(out_path) or os.path.getsize(out_path) == 0:
            return fail("no_report",
                        "msoffcrypto-tool не создал расшифрованный файл")

    return ok({"file": target, "decrypted": True, "output_path": out_path})


if __name__ == "__main__":
    sys.exit(main())