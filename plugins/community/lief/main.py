#!/usr/bin/env python3
"""lief — универсальный разбор бинарников (donor: LIEF, библиотека без CLI).

Вход (stdin JSON): file (путь к PE/ELF/Mach-O, обязателен), wall_timeout (опц.,
300).

Вызов: <LIEF_BIN|python3 -c SNIPPET> <абс.путь к файлу>  (cwd = временная папка).
SNIPPET — константа модуля: импортирует lief, зовёт общий парсер LIEF
(lief.parse, а не lief.ELF/lief.PE — назначение плагина именно общий разбор),
печатает один JSON в stdout. main.py саму lief не импортирует (только stdlib) и
никак в сеть не ходит.

Формат определяется и через lief.is_pe/is_elf/is_macho, и через
lief.parse: если файл не опознан ни одним из них — доменная ошибка
unsupported_format, если опознан, но не разобран — bad_file. Сниппет печатает
и class/секции: sections ограничены SECTION_LIMIT, count — полное число секций.

Протокол донора в сниппете: exit 0 и JSON в stdout — успех; exit 1 и
{"wedra_error": "<код>", "wedra_message": "..."} в stdout — доменная ошибка
(unsupported_format, bad_file, lief_not_installed).

Выход (stdout JSON): {file, format, header{architecture, object_type,
endianness, is_64, entrypoint}, sections[{name,size,offset,virtual_address}],
count}. На старых LIEF без абстрактного header поля остаются пустыми — разбор не
падает. Доменные ошибки: empty_file, missing_file, unsupported_format,
bad_file, lief_not_installed, timeout (retryable), no_report, tool_failed.
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

SECTION_LIMIT = 200


def bail(code, message):
    sys.stdout.write(json.dumps({"wedra_error": code,
                                 "wedra_message": message}))
    sys.exit(1)


def enum_name(value):
    name = getattr(value, "name", None)
    if name:
        return str(name)
    text = str(value)
    return text.rsplit(".", 1)[-1]


try:
    import lief
except ImportError as exc:
    bail("lief_not_installed",
         "\\u043f\\u0430\\u043a\\u0435\\u0442 lief \\u043d\\u0435 \\u0438\\u043c\\u043f\\u043e\\u0440\\u0442\\u0438\\u0440\\u0443\\u0435\\u0442\\u0441\\u044f: %s" % exc)

path = sys.argv[1]

try:
    known = bool(lief.is_pe(path) or lief.is_elf(path) or lief.is_macho(path))
except Exception:
    known = False

try:
    target = lief.parse(path)
except Exception:
    target = None

if target is None:
    if known:
        bail("bad_file", "LIEF \\u043d\\u0435 \\u0441\\u043c\\u043e\\u0433 \\u0440\\u0430\\u0437\\u043e\\u0431\\u0440\\u0430\\u0442\\u044c \\u0444\\u0430\\u0439\\u043b: %s" % path)
    bail("unsupported_format",
         "LIEF \\u043d\\u0435 \\u043f\\u043e\\u0434\\u0434\\u0435\\u0440\\u0436\\u0438\\u0432\\u0430\\u0435\\u0442 \\u0444\\u043e\\u0440\\u043c\\u0430\\u0442 \\u0444\\u0430\\u0439\\u043b\\u0430 (ожидались PE, ELF \\u0438\\u043b\\u0438 Mach-O): %s" % path)

abstract = getattr(target, "abstract", target)

try:
    entrypoint = int(abstract.entrypoint)
except Exception:
    entrypoint = 0

header = {"architecture": "", "object_type": "", "endianness": "",
          "is_64": False, "entrypoint": entrypoint}
try:
    raw = abstract.header
    header = {
        "architecture": enum_name(raw.architecture),
        "object_type": enum_name(raw.object_type),
        "endianness": enum_name(raw.endianness),
        "is_64": bool(raw.is_64),
        "entrypoint": entrypoint,
    }
except Exception:
    pass

sections = []
try:
    for section in abstract.sections:
        if len(sections) >= SECTION_LIMIT:
            break
        name = section.name
        if isinstance(name, bytes):
            name = name.decode("utf-8", "replace")
        sections.append({"name": str(name or ""),
                         "size": int(section.size),
                         "offset": int(section.offset),
                         "virtual_address": int(section.virtual_address)})
except Exception:
    pass

try:
    section_count = len(abstract.sections)
except Exception:
    section_count = len(sections)

print(json.dumps({"format": enum_name(getattr(abstract, "format",
                                              "UNKNOWN")).upper(),
                  "header": header, "sections": sections,
                  "section_count": int(section_count)}))
'''

DONOR_ERRORS = ("unsupported_format", "bad_file", "lief_not_installed")

HEADER_DEFAULTS = {"architecture": "", "object_type": "", "endianness": "",
                   "is_64": False, "entrypoint": 0}


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
    bin_env = os.environ.get("LIEF_BIN", "").strip()
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


def normalize_header(value):
    raw = value if isinstance(value, dict) else {}
    header = dict(HEADER_DEFAULTS)
    for key in ("architecture", "object_type", "endianness"):
        header[key] = str(raw.get(key) or "")
    header["is_64"] = bool(raw.get("is_64"))
    try:
        header["entrypoint"] = int(raw.get("entrypoint") or 0)
    except (TypeError, ValueError):
        header["entrypoint"] = 0
    return header


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
            return fail("lief_not_installed",
                        "lief не запустился: pip install lief "
                        "(или укажите LIEF_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"разбор lief не уложился в {wall:.0f}s: "
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
                        f"lief вернул код {proc.returncode}: {last}")
        if not stdout.strip():
            return fail("no_report", "сниппет lief не напечатал JSON")
        if report is None:
            return fail("bad_report", "не разобран JSON-сниппет lief",
                        exit_code=2)

    marker = donor_error(report)
    if marker:
        return fail(str(marker["wedra_error"]),
                    str(marker.get("wedra_message") or target))
    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат ответа lief",
                    exit_code=2)

    sections = [{"name": str(s.get("name") or ""),
                 "size": int(s.get("size") or 0),
                 "offset": int(s.get("offset") or 0),
                 "virtual_address": int(s.get("virtual_address") or 0)}
                for s in pick_dicts(report.get("sections"))]
    try:
        count = int(report.get("section_count"))
    except (TypeError, ValueError):
        count = len(sections)

    return ok({"file": target,
               "format": str(report.get("format") or "UNKNOWN").upper(),
               "header": normalize_header(report.get("header")),
               "sections": sections, "count": count})


if __name__ == "__main__":
    sys.exit(main())