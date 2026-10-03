#!/usr/bin/env python3
"""cloud_enum — поиск облачных хранилищ по ключу/имени (обёртка над CLI cloud_enum).

Вход (stdin JSON): key (опц.), name (опц.), mutations (опц., путь к -m и -b),
quickscan (опц., bool), wall_timeout (опц., общий лимит, 300). Ключевые слова
берутся ИЗ ВХОДА, не из env, поэтому секретов плагин не читает (secrets: []).
Обязателен хотя бы один из key/name — иначе инструмент падает на argparse:
доменная ошибка empty_input_args.

Вызов: <CLOUD_ENUM_BIN|cloud_enum|python3 -m cloud_enum> -k <key> [-k <name>]
       -l cloud_enum.json -f json [-m <mutations> -b <mutations>] [-qs]
       (cwd = временная папка).

Точка входа. Donor — репозиторий RHISAC/cloud_enum, ныне initstring/cloud_enum
(версия 0.8 в pyproject.toml), верхнеуровневый модуль cloud_enum.py с
[project.scripts] cloud_enum = "cloud_enum:main", поэтому работают и консольный
скрипт `cloud_enum`, и `python -m cloud_enum`. Флаги сверены с parse_arguments()
(0.8): -k/--keyword (action='append', группа required и mutually exclusive с
-kf/--keyfile — поэтому два наших слова идут двумя -k, это документированный
способ «provide multiple keywords»), -l/--logfile, -f/--format (text|json|csv),
-m/--mutations, -qs/--quickscan, --disable-aws/-azure/-gcp. Флага DigitalOcean у
текущего донора нет: апстрим проверяет AWS/Azure/GCP; provider берётся из platform
самого инструмента, так что сборка с DigitalOcean пройдёт как есть.

Список мутаций. parse_arguments() ДО любой работы проверяет os.access() на ДВА
файла: mutations (-m) и brute (-b), оба по умолчанию
script_path + '/enum_tools/fuzz.txt', где script_path — каталог argv[0]. У
консольного скрипта это venv/bin (venv/Scripts), а не site-packages, поэтому оба
дефолта недоступны и cloud_enum выходит с кодом 0 ДО создания лога: «[!] Cannot
access mutations file» / «[!] Cannot read brute-force file, exiting». Отсюда два
правила: (1) если оператор дал mutations, мы отдаём его и в -m, и в -b — список
мутаций годится и как brute-лист (ровно этим же файлом донора заполняются оба
дефолта), иначе прогон умирает на проверке -b; (2) mutations стоит задавать
всегда: wheel из pip вообще не везёт fuzz.txt (в [tool.setuptools] нет
package-data, в site-packages лежат только .py), так что `pip install
cloud_enum` нерабочий ещё и без этого. На PyPI имя cloud-enum помечено
quarantined и файлов не содержит — ставят из git: pip install
git+https://github.com/initstring/cloud_enum (или клон и запуск из корня).

Отчёт: enum_tools/utils.py init_logfile пишет заголовок "\n\n#### CLOUD_ENUM
<дата> ####\n", дальше fmt_output при LOGFILE_FMT=json дописывает по строке
JSON: {"platform": "aws", "msg": "OPEN S3 BUCKET", "target": "http://...",
"access": "public"} — access бывает public|protected|disabled. Заголовок
пропускаем, любая другая неразбираемая строка — bad_report (exit 2). Пустой лог
(только заголовок) = ничего не нашлось → ok с пустым массивом.

Выход (stdout JSON): {bucket_name, provider, open, services, count}.
bucket_name/provider — хост и платформа ПЕРВОГО открытого ресурса, а если
открытых нет — первого найденного ("" если находок нет); open — есть ли хоть один
access=public; services — отсортированный список платформ, где что-то нашлось
(строки как их отдал инструмент: aws/azure/gcp); count = len(services).

Доменные ошибки: empty_input_args, bad_quickscan, cloud_enum_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON
входа, нечисловой wall_timeout, нечитаемый лог (bad_report).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
except Exception:
    pass

DEFAULT_WALL = 300
REPORT_NAME = "cloud_enum.json"


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def as_bool(value, default):
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        low = value.strip().lower()
        if low in ("1", "true", "yes", "on"):
            return True
        if low in ("", "0", "false", "no", "off"):
            return False
        return default
    return default


def resolve_bin():
    bin_env = os.environ.get("CLOUD_ENUM_BIN", "").strip()
    if bin_env:
        if "/" not in bin_env and "\\" not in bin_env:
            return [shutil.which(bin_env) or os.path.abspath(bin_env)]
        return [os.path.abspath(bin_env)]
    found = shutil.which("cloud_enum")
    if found:
        return [found]
    return [sys.executable, "-m", "cloud_enum"]


def resource_name(target):
    text = str(target or "").strip()
    if not text:
        return ""
    host = urlsplit(text).hostname or ""
    if not host:
        host = text.split("//")[-1].split("/")[0]
    return host.strip().rstrip(".").lower()


def parse_log(text):
    findings = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("####"):
            continue
        try:
            record = json.loads(line)
        except ValueError as e:
            raise ValueError(f"строка лога не JSON ({e}): {line[:120]}")
        if not isinstance(record, dict):
            raise ValueError("строка лога не объект: " + line[:120])
        provider = str(record.get("platform") or "").strip().lower()
        name = resource_name(record.get("target"))
        if not provider and not name:
            continue
        findings.append({
            "provider": provider,
            "name": name,
            "open": str(record.get("access") or "").strip().lower()
            == "public",
        })
    return findings


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    key = str(data.get("key") or "").strip()
    name = str(data.get("name") or "").strip()
    if not key and not name:
        return fail("empty_input_args",
                    "нужен хотя бы один из ключей: key или name "
                    "(cloud_enum требует -k/--keyword)")

    mutations = str(data.get("mutations") or "").strip()
    if data.get("quickscan") is not None and not isinstance(
            data.get("quickscan"), (bool, str, int, float)):
        return fail("bad_quickscan", "quickscan должен быть boolean")
    quickscan = as_bool(data.get("quickscan"), False)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (см. holehe)
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, REPORT_NAME)
        for word in (key, name):
            if word:
                cmd += ["-k", word]
        cmd += ["-l", report_path, "-f", "json"]
        if mutations:
            cmd += ["-m", mutations, "-b", mutations]
        if quickscan:
            cmd.append("-qs")

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall,
                                  stdin=subprocess.DEVNULL)
        except FileNotFoundError:
            return fail("cloud_enum_not_installed",
                        "cloud_enum не найден: pip install cloud_enum "
                        "(или укажите CLOUD_ENUM_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"cloud_enum не уложился в {wall:.0f}s: включите "
                        "quickscan или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if not os.path.isfile(report_path):
            tail = combined.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"cloud_enum упал (exit {proc.returncode}): {last}")
            low = combined.lower()
            if "cannot access" in low or "brute-force file" in low:
                return fail("no_report",
                            f"cloud_enum не начал перебор ({last}); нужен "
                            "список мутаций — передайте вход mutations "
                            "(путь к enum_tools/fuzz.txt из клона "
                            "github.com/initstring/cloud_enum: в wheel из "
                            "pip этого файла нет)")
            return fail("no_report", f"cloud_enum не дал лог -l: {last}")
        try:
            with open(report_path, encoding="utf-8") as f:
                text = f.read()
        except Exception as e:
            return fail("bad_report", f"не прочитан лог cloud_enum: {e}",
                        exit_code=2)

    try:
        findings = parse_log(text)
    except ValueError as e:
        return fail("bad_report", str(e), exit_code=2)

    open_findings = [f for f in findings if f["open"]]
    primary = open_findings[0] if open_findings else (findings[0]
                                                       if findings else None)
    services = []
    for item in findings:
        if item["provider"] and item["provider"] not in services:
            services.append(item["provider"])
    services.sort()

    return ok({"bucket_name": primary["name"] if primary else "",
               "provider": primary["provider"] if primary else "",
               "open": bool(open_findings),
               "services": services,
               "count": len(services)})


if __name__ == "__main__":
    sys.exit(main())