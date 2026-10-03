#!/usr/bin/env python3
"""binwalk — сигнатуры и извлечение данных из бинарника (обёртка над CLI binwalk).

Вход (stdin JSON): file (путь к бинарнику), extract (опц., bool), wall_timeout
(опц., общий лимит, 300).

Вызов: <BINWALK_BIN|binwalk> --log <tmp>/binwalk_report.json --quiet
       [--extract --directory <tmp>/extractions] <файл>  (cwd = временная папка).

Флаги сверены с src/cliparser.rs binwalk v3.1.0 (clap): -l/--log — JSON-отчёт в
файл ('-' — stdout), -q/--quiet — не печатать таблицу, -e/--extract — извлекать,
-C/--directory — каталог извлечения (дефолт v3 "extractions" относительно CWD),
-M/--matryoshka — рекурсия. Плагин зовёт длинные имена, поэтому короткий `-C`
не используется. Каталог извлечения всегда под temp-CWD, рядом с исходным
файлом binwalk ничего не пишет. В v2 флагов --log/--directory нет — плагин
требует именно v3, а `pip install binwalk` ставит нерабочий sdist 2.1.0 от
2015 года (в нём нет пакета binwalk.core), v3 — это Rust-бинарь из cargo/Docker.

Формат отчёта сверен с src/json.rs и src/binwalk.rs: JSON-массив объектов
{"Analysis": {file_path, file_map[...], extractions{id: {...}}}}, где запись
file_map — {offset, id, size, name, confidence, description, always_display,
extraction_declined}, а extractions — {size, success, extractor,
do_not_recurse, output_directory}. Пустой отчёт (ничего не нашли) — это ok с
пустым signatures, а не ошибка.

Выход (stdout JSON): {file, signatures[{offset,size,name,description,
confidence,extracted}], extracted[], count}. extracted — реальные файлы из
каталогов успешных extractions, пути относительно каталога извлечения.
Доменные ошибки: empty_file, missing_file, binwalk_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
неверные типы полей, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300
REPORT_NAME = "binwalk_report.json"
EXTRACT_DIR = "extractions"

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


def resolve_bin():
    bin_env = os.environ.get("BINWALK_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("binwalk")
    if found:
        return [found]
    return [sys.executable, "-m", "binwalk"]


def to_int(value):
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def collect(report, extract_root):
    """Разбор JSON-отчёта binwalk v3 → (signatures, extracted)."""
    signatures = []
    extracted = []
    for entry in report:
        if not isinstance(entry, dict):
            continue
        analysis = entry.get("Analysis")
        if not isinstance(analysis, dict):
            continue
        extractions = analysis.get("extractions")
        extractions = extractions if isinstance(extractions, dict) else {}
        for sig in analysis.get("file_map") or []:
            if not isinstance(sig, dict):
                continue
            extraction = extractions.get(sig.get("id"))
            extracted_flag = bool(isinstance(extraction, dict)
                                  and extraction.get("success"))
            signatures.append({
                "offset": to_int(sig.get("offset")),
                "size": to_int(sig.get("size")),
                "name": str(sig.get("name") or ""),
                "description": str(sig.get("description") or ""),
                "confidence": to_int(sig.get("confidence")),
                "extracted": extracted_flag,
            })
            if not extracted_flag:
                continue
            outdir = str(extraction.get("output_directory") or "")
            if not outdir or not os.path.isdir(outdir):
                continue
            for root, _dirs, names in os.walk(outdir):
                for name in names:
                    full = os.path.join(root, name)
                    if os.path.islink(full) or not os.path.isfile(full):
                        continue
                    rel = os.path.relpath(full, extract_root)
                    rel = rel.replace(os.sep, "/")
                    if not rel.startswith(".."):
                        extracted.append(rel)
    return signatures, sorted(set(extracted))


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

    extract = data.get("extract")
    if extract is None:
        extract = False
    elif not isinstance(extract, bool):
        return fail("bad_extract", "extract обязан быть boolean", exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        extract_root = os.path.join(td, EXTRACT_DIR)
        cmd += ["--log", report_path, "--quiet"]
        if extract:
            cmd += ["--extract", "--directory", extract_root]
        cmd += [path]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("binwalk_not_installed",
                        "binwalk не найден: нужен binwalk v3 (--log/--directory), "
                        "это Rust-бинарь — cargo install binwalk или Docker-образ, "
                        "pip даёт нерабочий sdist 2.1.0; путь — в BINWALK_BIN")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"binwalk не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        tail = (proc.stderr or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        blob = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if NOT_MODULE_RE.search(blob):
            return fail("binwalk_not_installed",
                        "binwalk не установлен: pip install binwalk "
                        "не годится (на PyPI только нерабочий sdist 2.1.0), "
                        "нужен v3 из cargo/Docker (или укажите BINWALK_BIN)")
        if proc.returncode != 0:
            return fail("tool_failed",
                        f"binwalk упал (exit {proc.returncode}): {last}")
        if not os.path.isfile(report_path):
            return fail("no_report",
                        f"binwalk не дал JSON-отчёт (нужен binwalk v3 с "
                        f"--log): {last}")
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON binwalk: {e}",
                        exit_code=2)
        if not isinstance(report, list):
            return fail("bad_report", "ожидался массив в отчёте binwalk",
                        exit_code=2)
        signatures, extracted = collect(report, extract_root)

    return ok({"file": path, "signatures": signatures,
               "extracted": extracted, "count": len(signatures)})


if __name__ == "__main__":
    sys.exit(main())
