#!/usr/bin/env python3
"""pefile — разбор PE-файла (donor: pefile, библиотека без CLI, паттерн C).

Вход (stdin JSON): file (путь к PE-файлу, обязателен), wall_timeout (опц., 300).

Вызов: <PEFILE_BIN|python3 -c SNIPPET> <абс.путь к файлу>  (cwd = временная папка).
SNIPPET — константа модуля: импортирует pefile, читает заголовки, таблицу
импортов и печатает один компактный JSON в stdout. main.py саму pefile не
импортирует (только stdlib) и никак в сеть не ходит.

Протокол донора в сниппете: exit 0 и JSON в stdout — успех; exit 1 и
{"wedra_error": "<код>", "wedra_message": "..."} в stdout — доменная ошибка
(not_pe, bad_file, pefile_not_installed).

Выход (stdout JSON): {file, machine, sections[{name,virtual_address,size,
characteristics}], imports[{dll,symbols[]}], count}, где count — число
импортируемых DLL. machine — IMAGE_FILE_MACHINE_* по значению FILE_HEADER.Machine.
Доменные ошибки: empty_file, missing_file, not_pe, bad_file,
pefile_not_installed, timeout (retryable), no_report, tool_failed.
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


def bail(code, message):
    sys.stdout.write(json.dumps({"wedra_error": code,
                                 "wedra_message": message}))
    sys.exit(1)


try:
    import pefile
except ImportError as exc:
    bail("pefile_not_installed",
         "\\u043f\\u0430\\u043a\\u0435\\u0442 pefile \\u043d\\u0435 \\u0438\\u043c\\u043f\\u043e\\u0440\\u0442\\u0438\\u0440\\u0443\\u0435\\u0442\\u0441\\u044f: %s" % exc)

path = sys.argv[1]
try:
    pe = pefile.PE(path, fast_load=True)
except pefile.PEFormatError as exc:
    bail("not_pe",
         "\\u0444\\u0430\\u0439\\u043b \\u043d\\u0435 \\u0447\\u0438\\u0442\\u0430\\u0435\\u0442\\u0441\\u044f \\u043a\\u0430\\u043a PE: %s" % exc)
except Exception as exc:
    bail("bad_file",
         "\\u043d\\u0435 \\u0443\\u0434\\u0430\\u043b\\u043e\\u0441\\u044c \\u0440\\u0430\\u0437\\u043e\\u0431\\u0440\\u0430\\u0442\\u044c PE: %r" % (exc,))

try:
    pe.parse_data_directories(
        directories=[pefile.DIRECTORY_ENTRY["IMAGE_DIRECTORY_ENTRY_IMPORT"]])
    sections = []
    for section in pe.sections:
        raw = section.Name.split(b"\\x00")[0]
        sections.append({
            "name": raw.decode("ascii", "replace"),
            "virtual_address": int(section.VirtualAddress),
            "size": int(section.SizeOfRawData or 0),
            "characteristics": int(section.Characteristics or 0),
        })
    imports = []
    for entry in getattr(pe, "DIRECTORY_ENTRY_IMPORT", []):
        symbols = []
        for symbol in entry.imports:
            if symbol.name:
                symbols.append(symbol.name.decode("ascii", "replace"))
            elif symbol.ordinal:
                symbols.append("#%d" % int(symbol.ordinal))
        imports.append({"dll": entry.dll.decode("ascii", "replace"),
                        "symbols": symbols})
    print(json.dumps({"machine": int(pe.FILE_HEADER.Machine),
                      "sections": sections, "imports": imports}))
finally:
    pe.close()
'''

MACHINE_NAMES = {
    0x014C: "IMAGE_FILE_MACHINE_I386",
    0x0200: "IMAGE_FILE_MACHINE_IA64",
    0x01C0: "IMAGE_FILE_MACHINE_ARM",
    0x01C4: "IMAGE_FILE_MACHINE_ARMNT",
    0x0EBC: "IMAGE_FILE_MACHINE_EBC",
    0x5032: "IMAGE_FILE_MACHINE_RISCV32",
    0x5064: "IMAGE_FILE_MACHINE_RISCV64",
    0x6264: "IMAGE_FILE_MACHINE_LOONGARCH64",
    0x8664: "IMAGE_FILE_MACHINE_AMD64",
    0xAA64: "IMAGE_FILE_MACHINE_ARM64",
}

DONOR_ERRORS = ("not_pe", "bad_file", "pefile_not_installed")


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
    bin_env = os.environ.get("PEFILE_BIN", "").strip()
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


def machine_name(value):
    try:
        code = int(value)
    except (TypeError, ValueError):
        return ""
    return MACHINE_NAMES.get(code, "0x%04x" % code)


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
            return fail("pefile_not_installed",
                        "pefile не запустился: pip install pefile "
                        "(или укажите PEFILE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"разбор pefile не уложился в {wall:.0f}s: "
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
                        f"pefile вернул код {proc.returncode}: {last}")
        if not stdout.strip():
            return fail("no_report", "сниппет pefile не напечатал JSON")
        if report is None:
            return fail("bad_report", "не разобран JSON-сниппет pefile",
                        exit_code=2)

    marker = donor_error(report)
    if marker:
        return fail(str(marker["wedra_error"]),
                    str(marker.get("wedra_message") or target))
    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат ответа pefile",
                    exit_code=2)

    sections = [{"name": str(s.get("name") or ""),
                 "virtual_address": int(s.get("virtual_address") or 0),
                 "size": int(s.get("size") or 0),
                 "characteristics": int(s.get("characteristics") or 0)}
                for s in pick_dicts(report.get("sections"))]
    imports = [{"dll": str(entry.get("dll") or ""),
                "symbols": [str(x) for x in (entry.get("symbols") or [])]}
               for entry in pick_dicts(report.get("imports"))]

    return ok({"file": target, "machine": machine_name(report.get("machine")),
               "sections": sections, "imports": imports,
               "count": len(imports)})


if __name__ == "__main__":
    sys.exit(main())