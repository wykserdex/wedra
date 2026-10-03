#!/usr/bin/env python3
"""pyelftools — разбор ELF-файла (donor: elftools, библиотека без CLI, паттерн C).

Вход (stdin JSON): file (путь к ELF-файлу, обязателен), wall_timeout (опц., 300).

Вызов: <PYELFTOOLS_BIN|python3 -c SNIPPET> <абс.путь к файлу>  (cwd = временная
папка). SNIPPET — константа модуля: импортирует elftools, читает class,
machine, type, entry, секции, символы из .dynsym/.symtab (не больше
SYMBOL_LIMIT) и список DT_NEEDED, печатает один JSON в stdout. DT_NEEDED берём
перебором секций: у секции с типом SHT_DYNAMIC (имя .dynamic, не .dynlink) есть
iter_tags(), и у тегов с d_tag=DT_NEEDED есть поле needed. main.py саму elftools
не импортирует (только stdlib) и никак в сеть не ходит.

Протокол донора в сниппете: exit 0 и JSON в stdout — успех; exit 1 и
{"wedra_error": "<код>", "wedra_message": "..."} в stdout — доменная ошибка
(not_elf, bad_file, pyelftools_not_installed).

Выход (stdout JSON): {file, elf_class, sections[{name,type,size,addr}],
libraries[], count}, где elf_class — "ELF32"/"ELF64", libraries — DT_NEEDED,
count — число секций. Сниппет отдаёт ещё machine/type/entry/symbols — в
манифесте они не объявлены и наружу не возвращаются.
Доменные ошибки: empty_file, missing_file, not_elf, bad_file,
pyelftools_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечитаемый stdout донора.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

SNIPPET = '''\
import json, sys

SYMBOL_LIMIT = 200


def bail(code, message):
    sys.stdout.write(json.dumps({"wedra_error": code,
                                 "wedra_message": message}))
    sys.exit(1)


try:
    from elftools.elf.elffile import ELFFile
except ImportError as exc:
    bail("pyelftools_not_installed",
         "\\u043f\\u0430\\u043a\\u0435\\u0442 pyelftools \\u043d\\u0435 \\u0438\\u043c\\u043f\\u043e\\u0440\\u0442\\u0438\\u0440\\u0443\\u0435\\u0442\\u0441\\u044f: %s" % exc)

try:
    from elftools.common.exceptions import ELFError
except ImportError:
    ELFError = Exception

path = sys.argv[1]
try:
    handle = open(path, "rb")
except OSError as exc:
    bail("bad_file", "\\u043d\\u0435 \\u0443\\u0434\\u0430\\u043b\\u043e\\u0441\\u044c \\u043e\\u0442\\u043a\\u0440\\u044b\\u0442\\u044c \\u0444\\u0430\\u0439\\u043b: %r" % (exc,))

try:
    elf = ELFFile(handle)
except ELFError as exc:
    handle.close()
    bail("not_elf", "\\u0444\\u0430\\u0439\\u043b \\u043d\\u0435 \\u0447\\u0438\\u0442\\u0430\\u0435\\u0442\\u0441\\u044f \\u043a\\u0430\\u043a ELF: %s" % exc)
except Exception as exc:
    handle.close()
    bail("bad_file", "\\u043d\\u0435 \\u0443\\u0434\\u0430\\u043b\\u043e\\u0441\\u044c \\u0440\\u0430\\u0437\\u043e\\u0431\\u0440\\u0430\\u0442\\u044c ELF: %r" % (exc,))

try:
    sections = []
    libraries = []
    symbols = []
    for section in elf.iter_sections():
        sections.append({
            "name": str(section.name or ""),
            "type": str(section["sh_type"]),
            "size": int(section["sh_size"]),
            "addr": int(section["sh_addr"]),
        })
        iter_tags = getattr(section, "iter_tags", None)
        if iter_tags is not None:
            for tag in iter_tags():
                needed = getattr(tag, "needed", None)
                if needed and needed not in libraries:
                    libraries.append(str(needed))
        if len(symbols) < SYMBOL_LIMIT and section.name in (".dynsym", ".symtab"):
            for symbol in section.iter_symbols():
                if len(symbols) >= SYMBOL_LIMIT:
                    break
                if not symbol.name:
                    continue
                symbols.append({
                    "name": str(symbol.name),
                    "type": str(symbol["st_info"]["type"]),
                    "value": int(symbol["st_value"]),
                    "size": int(symbol["st_size"]),
                })
    print(json.dumps({
        "class": "ELF%d" % int(elf.elfclass),
        "machine": str(elf.header["e_machine"]),
        "type": str(elf.header["e_type"]),
        "entry": int(elf.header["e_entry"]),
        "sections": sections,
        "symbols": symbols,
        "imported_libraries": libraries,
    }))
finally:
    handle.close()
'''

DONOR_ERRORS = ("not_elf", "bad_file", "pyelftools_not_installed")


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


def resolve_cmd():
    bin_env = os.environ.get("PYELFTOOLS_BIN", "").strip()
    if not bin_env:
        return [sys.executable, "-c", SNIPPET]
    if "/" in bin_env or "\\" in bin_env:
        cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    return cmd


def parse_json_blob(text):
    stripped = (text or "").strip()
    if not stripped:
        return None
    try:
        return json.loads(stripped)
    except ValueError:
        return None


def donor_error(report):
    if isinstance(report, dict) and report.get("wedra_error") in DONOR_ERRORS:
        return report
    return None


def pick_dicts(value):
    return [item for item in (value or []) if isinstance(item, dict)]


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    target = str(data.get("file") or "").strip()
    if not target:
        return fail("empty_file", "file пуст")
    file_path = os.path.abspath(os.path.expanduser(target))
    if not os.path.isfile(file_path):
        return fail("missing_file", f"файл не найден: {target}")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_cmd() + [file_path]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  errors="replace", cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("pyelftools_not_installed",
                        "pyelftools не запустился: pip install pyelftools "
                        "(или укажите PYELFTOOLS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"разбор pyelftools не уложился в {wall:.0f}s: "
                        "увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = proc.stdout or ""
        report = parse_json_blob(stdout)
        if proc.returncode != 0:
            marker = donor_error(report)
            if marker:
                return fail(str(marker["wedra_error"]),
                            str(marker.get("wedra_message") or target))
            tail = (proc.stderr or stdout or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"pyelftools вернул код {proc.returncode}: {last}")
        if not stdout.strip():
            return fail("no_report", "сниппет pyelftools не напечатал JSON")
        if report is None:
            return fail("bad_report", "не разобран JSON-сниппет pyelftools",
                        exit_code=2)

    marker = donor_error(report)
    if marker:
        return fail(str(marker["wedra_error"]),
                    str(marker.get("wedra_message") or target))
    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат ответа pyelftools",
                    exit_code=2)

    sections = [{"name": str(s.get("name") or ""),
                 "type": str(s.get("type") or ""),
                 "size": int(s.get("size") or 0),
                 "addr": int(s.get("addr") or 0)}
                for s in pick_dicts(report.get("sections"))]
    libraries = [str(lib) for lib in (report.get("imported_libraries") or [])
                 if isinstance(lib, (str, bytes))]

    return ok({"file": target,
               "elf_class": str(report.get("class") or ""),
               "sections": sections, "libraries": libraries,
               "count": len(sections)})


if __name__ == "__main__":
    sys.exit(main())