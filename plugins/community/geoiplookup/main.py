#!/usr/bin/env python3
"""geoiplookup — GeoIP по IP через CLI geoiplookup (локальная база).

Вход (stdin JSON): ip, db_path (опц., файл или каталог базы),
wall_timeout (опц., общий лимит, 60).

Вызов: <GEOIPLOOKUP_BIN|geoiplookup> (-f <файл базы> | -d <каталог>) <ip>
       (cwd = временная папка).

Инструмент с именем geoiplookup документирован (manpage geoip-bin, linux.die.net):
`geoiplookup [-d directory] [-f filename] [-v] <ipaddress|hostname>`, по
умолчанию база ищется в /usr/share/GeoIP, ответ печатается одной строкой вида
`NL, Netherlands`. Никаких других флагов и никакого JSON-вывода у него нет,
поэтому страна разбирается из этой строки (паттерн B, текст), а region/city
берутся из помеченных строк `Region:`/`City:`, если их печатает сборка с
City-базой. Дона из ТЗ (speciallicity/geoiplookup, pip install geoiplookup)
не существует — ни репозитория, ни пакета; взят документированный инструмент
с этим именем.

Путь к базе: вход db_path, иначе env GEOIP_DB_PATH. Нет пути или пути на диске
нет — доменная ошибка missing_db (retryable: false). Файл отдаётся флагом -f,
каталог — -d; оба флага документированы.

Выход (stdout JSON): {ip, country, region, city, found}. found — страна
найдена. Ничего не нашлось — ok с пустыми строками и found=false. Доменные
ошибки: empty_ip, bad_ip, missing_db, geoiplookup_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
нечисловой wall_timeout.
"""
import ipaddress
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 60
DEFAULT_BIN = "geoiplookup"

# Строка ответа MaxMind: "NL, Netherlands".
COUNTRY_LINE_RE = re.compile(r"^([A-Za-z]{2})\s*,\s*(.+)$")
LABELS = ("country", "region", "city")


try:
    sys.stdin.reconfigure(encoding="utf-8", errors="replace")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
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


def resolve_bin():
    bin_env = os.environ.get("GEOIPLOOKUP_BIN", DEFAULT_BIN).strip() or DEFAULT_BIN
    if "/" in bin_env or "\\" in bin_env:
        return [os.path.abspath(bin_env)]
    return [shutil.which(bin_env) or os.path.abspath(bin_env)]


def build_cmd(ip, db_flag, db_path):
    cmd = resolve_bin()
    if cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += [db_flag, db_path, ip]
    return cmd


def parse_fields(stdout):
    country = region = city = ""
    for raw in stdout.splitlines():
        line = raw.strip()
        if not line:
            continue
        if ":" in line:
            label, _, value = line.partition(":")
            key = label.strip().lower()
            value = value.strip()
            if key in LABELS and value:
                if key == "country" and not country:
                    country = value
                elif key == "region" and not region:
                    region = value
                elif key == "city" and not city:
                    city = value
            continue
        if not country:
            match = COUNTRY_LINE_RE.match(line)
            if match:
                country = line
    return country, region, city


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    ip = str(data.get("ip") or "").strip()
    if not ip:
        return fail("empty_ip", "ip пуст")
    try:
        ipaddress.ip_address(ip)
    except ValueError:
        return fail("bad_ip", f"ip {ip!r} не IPv4/IPv6 адрес")

    db_path = str(data.get("db_path") or "").strip()
    if not db_path:
        db_path = os.environ.get("GEOIP_DB_PATH", "").strip()
    if not db_path:
        return fail("missing_db",
                    "не задан путь к базе GeoIP: передайте db_path или "
                    "экспортируйте GEOIP_DB_PATH")
    if os.path.isdir(db_path):
        db_flag = "-d"
    elif os.path.isfile(db_path):
        db_flag = "-f"
    else:
        return fail("missing_db",
                    f"база GeoIP по пути {db_path!r} не найдена")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = build_cmd(ip, db_flag, db_path)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("geoiplookup_not_installed",
                        "geoiplookup не найден: apt install geoip-bin "
                        "(или укажите GEOIPLOOKUP_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"geoiplookup не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = (proc.stdout or "").strip()
        tail = [ln for ln in (proc.stderr or "").strip().splitlines()
                if ln.strip()]
        last = tail[-1] if tail else f"exit {proc.returncode}"

        if proc.returncode != 0:
            return fail("tool_failed", f"geoiplookup упал: {last}")
        if not stdout:
            return fail("no_report", "geoiplookup ничего не напечатал")

    country, region, city = parse_fields(stdout)
    return ok({"ip": ip, "country": country, "region": region, "city": city,
               "found": bool(country)})


if __name__ == "__main__":
    sys.exit(main())
