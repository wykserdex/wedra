#!/usr/bin/env python3
"""dnsrecon — DNS-разведка домена (обёртка над CLI dnsrecon).

Вход (stdin JSON): domain, types[] (опц., ["std"]), dictionary (опц., путь к
словарю — сам добавляет тип brt), wall_timeout (опц., 300).

Вызов: <DNSRECON_BIN|python3 -m dnsrecon> -d <domain> -t <t1,t2>
       [-D <словарь>] -j <tmp>/dnsrecon_report.json   (cwd = временная папка).

Ключ -j/--json у dnsrecon принимает ИМЯ ФАЙЛА (не булев флаг), stdout-режима
JSON нет: репорт всегда пишется на диск. Значения -t/--type — только ключи
type_map донора (std, rvl, brt, srv, axfr, bing, yand, crt, snoop, tld,
zonewalk); типа `spf` среди них нет, обратный резолв из SPF-текста включается
отдельным булевым флагом `-s`. Формат репорта — список словарей-записей, поля
зависят от типа (SOA: mname/address, NS: target/address, A: name/address, MX:
name/exchange/address, CNAME: name/target/address, SPF/TXT: strings,
AXFR-служебные: zone_transfer/ns_server). Поэтому запись
нормализуется в {type, name, value}: name — первое непустое из
name/mname/exchange/target; value — первое непустое из
strings/data/target/exchange/address/extra, кроме уже взятого на name ключа.
Первый элемент списка — служебный {"type": "ScanInfo", "arguments": {...},
"date": ...}, он в records попадает как type=ScanInfo с пустыми name/value.
Пустой список — нормальный результат (records: [], total: 0), не ошибка.
Список верхнего уровня вида {"records": [...]} тоже принимается.

Выход (stdout JSON): {domain, records[{type, name, value}], total}.

Доменные ошибки: empty_domain, missing_file, dnsrecon_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
неизвестный тип перечисления, нечитаемый JSON-репорт.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TYPES = ["std"]
DEFAULT_WALL = 300

# Допустимые значения -t/--type — ровно ключи type_map в dnsrecon/__main__.py
# (проверено на 0.10.1: `python -m dnsrecon -d x -t spf` → "This type of scan is
# not in the list: spf", exit 1). Типа `spf` у донора НЕТ: обратный резолв
# диапазонов из SPF-текста включается булевым флагом `-s`, а не элементом -t.
KNOWN_TYPES = {"std", "brt", "srv", "axfr", "rvl", "bing", "yand",
               "crt", "snoop", "tld", "zonewalk"}

NAME_KEYS = ("name", "mname", "exchange", "target")
VALUE_KEYS = ("strings", "data", "target", "exchange", "address", "extra")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def normalize_record(item):
    if not isinstance(item, dict):
        return None
    rtype = str(item.get("type") or item.get("record_type") or "").strip()
    name = ""
    name_key = None
    for key in NAME_KEYS:
        value = item.get(key)
        if value is None or value == "":
            continue
        name = str(value).strip()
        name_key = key
        break
    value = ""
    for key in VALUE_KEYS:
        if key == name_key:
            continue
        raw = item.get(key)
        if raw is None or raw == "":
            continue
        value = str(raw).strip()
        break
    if not rtype and not name and not value:
        return None
    return {"type": rtype, "name": name, "value": value}


def extract_records(report):
    if isinstance(report, list):
        items = report
    elif isinstance(report, dict):
        items = None
        for key in ("records", "zone_data", "data"):
            if isinstance(report.get(key), list):
                items = report[key]
                break
        if items is None:
            items = []
            for value in report.values():
                if isinstance(value, list):
                    items.extend(value)
    else:
        raise ValueError("отчёт не массив и не объект")
    records = []
    for item in items:
        record = normalize_record(item)
        if record is not None:
            records.append(record)
    return records


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8")
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    domain = str(data.get("domain") or "").strip().lower().rstrip(".")
    if not domain:
        return fail("empty_domain", "domain пуст")

    types = data.get("types")
    if types is None:
        types = list(DEFAULT_TYPES)
    if not isinstance(types, list):
        return fail("bad_types", "types обязан быть массивом", exit_code=2)
    types = [str(t).strip().lower() for t in types if str(t).strip()]
    unknown = [t for t in types if t not in KNOWN_TYPES]
    if unknown:
        return fail("bad_types",
                    "неизвестные типы перечисления dnsrecon: "
                    + ",".join(unknown), exit_code=2)

    dictionary = str(data.get("dictionary") or "").strip()
    if dictionary:
        if not os.path.isfile(os.path.expanduser(dictionary)):
            return fail("missing_file", f"словарь не найден: {dictionary}")
        if "brt" not in types:
            types.append("brt")
    if not types:
        types = list(DEFAULT_TYPES)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("DNSRECON_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "dnsrecon"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, "dnsrecon_report.json")
        cmd += ["-d", domain, "-t", ",".join(types)]
        if dictionary:
            cmd += ["-D", os.path.abspath(os.path.expanduser(dictionary))]
        cmd += ["-j", report_path]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("dnsrecon_not_installed",
                        "dnsrecon не найден: pip install dnsrecon "
                        "(или укажите DNSRECON_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"dnsrecon не уложился в {wall:.0f}s: уменьшите "
                        "types/size словаря или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.stdout:
            sys.stderr.write(proc.stdout)

        if not os.path.exists(report_path):
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else ""
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"dnsrecon упал с кодом {proc.returncode}: {last}")
            return fail("no_report",
                        f"dnsrecon не дал JSON-репорт: {last or 'файл пуст'}")

        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON dnsrecon: {e}",
                        exit_code=2)

    try:
        records = extract_records(report)
    except ValueError as e:
        return fail("bad_report", f"неожиданный формат репорта dnsrecon: {e}",
                    exit_code=2)

    return ok({"domain": domain, "records": records, "total": len(records)})


if __name__ == "__main__":
    sys.exit(main())