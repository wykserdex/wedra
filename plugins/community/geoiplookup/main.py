#!/usr/bin/env python3
"""geoiplookup — GeoIP по IP через CLI geoiplookup (локальная база).

Вход (stdin JSON): ip, db_path (опц., файл или каталог базы),
wall_timeout (опц., общий лимит, 60).

Вызов: <GEOIPLOOKUP_BIN|geoiplookup> (-f <файл базы> | -d <каталог>) <ip>
       (cwd = временная папка).

Инструмент с именем geoiplookup — это C-утилита MaxMind из пакета geoip-bin;
сверено по исходнику maxmind/geoip-api-c (apps/geoiplookup.c,
man/geoiplookup.1.in, libGeoIP/GeoIP.c). Флаги там ровно `-h`, `-?`,
`-d <каталог>`, `-f <файл>`, `-v`, `-i`, `-l`: длинных --directory/--filename
нет. Путь к базе — только -f (один .dat) либо -d (каталог: инструмент сам
перебирает все найденные базы и печатает по строке на каждую), по умолчанию
DATADIR (/usr/share/GeoIP). Читает утилита только старые легаси-.dat
(GeoIP.dat, GeoIPCity.dat, GeoIPRegion.dat, GeoIPOrg.dat, GeoIPASNum.dat, ...)
и не открывает MMDB (GeoLite2-*.mmdb, DB-IP) — это mmdblookup, другой
инструмент. JSON-вывода нет, ответ — текст (паттерн B).

Ключевое: строка ответа печатается ВСЕГДА с подписью базы из GeoIPDBDescription
(`printf("%s: %s, %s\n", GeoIPDBDescription[i], code, name)`), а не голой
строкой `NL, Netherlands` — устаревшая manpage про голый вид забыта. Реальные
строки:
  GeoIP Country Edition: NL, Netherlands
  GeoIP Region Edition, Rev 1: NL, NH
  GeoIP City Edition, Rev 1: US, CA, California, Mountain View, 94043,
                        37.42, -122.08, 807, 0
  GeoIP Country Edition: IP Address not found
Поэтому страна/регион/город разбираются из хвоста после подписи: у Country-базы
country — строка `КС, Страна`, у Region-базы хвост `КС, РЕГИОН`, у City-базы
`КС, код региона, имя региона, город, индекс, ...`; `_mk_NA` печатает `N/A` —
считаем пустым значением. Строки `Country:`/`Region:`/`City:` (обёртки без
подписи базы) и голая строка `КС, Страна` тоже понимаются.
Дона из ТЗ (speciallicity/geoiplookup, pip install geoiplookup) не существует —
ни репозитория, ни пакета; взят документированный инструмент с этим именем.

Путь к базе: вход db_path, иначе env GEOIP_DB_PATH. Нет пути или пути на диске
нет — доменная ошибка missing_db (retryable: false). Файл отдаётся флагом -f,
каталог — -d; оба флага документированы исходником.

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

# Хвост строки ответа MaxMind: "NL, Netherlands".
COUNTRY_LINE_RE = re.compile(r"^[A-Za-z]{2}\s*,\s*.+$")
COUNTRY_CODE_RE = re.compile(r"^[A-Za-z]{2}$")
LABELS = ("country", "region", "city")
# Подписи баз из GeoIPDBDescription (libGeoIP/GeoIP.c) -> что из строки берём.
EDITION_LABELS = {
    "geoip country edition": "country",
    "geoip large country edition": "country",
    "geoip country v6 edition": "country",
    "geoip large country v6 edition": "country",
    "geoip region edition, rev 0": "region",
    "geoip region edition, rev 1": "region",
    "geoip city edition, rev 0": "city",
    "geoip city edition, rev 1": "city",
    "geoip city edition v6, rev 0": "city",
    "geoip city edition v6, rev 1": "city",
}
# _mk_NA() в geoiplookup.c печатает "N/A" вместо неизвестного значения.
NA = "N/A"


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


def edition_kind(label):
    """Что из строки берём по подписи базы: country/region/city или ''."""
    kind = EDITION_LABELS.get(label)
    if kind:
        return kind
    if not label.startswith("geoip "):
        return ""
    for word in ("country", "region", "city"):
        if word in label:
            return word
    return ""


def split_fields(value):
    """Хвост строки `КС, ...` разбит по запятым, "N/A" заменён пустой строкой."""
    return ["" if part.strip() == NA else part.strip()
            for part in value.split(",")]


def parse_edition(kind, value):
    """(страна, регион, город) из хвоста строки ответа с подписью базы."""
    parts = split_fields(value)
    if not parts or not COUNTRY_CODE_RE.match(parts[0]):
        return "", "", ""
    if kind == "country":
        # "NL, Netherlands"
        return (value if COUNTRY_LINE_RE.match(value) else ""), "", ""
    if kind == "region":
        # "NL, NH"
        return parts[0], (parts[1] if len(parts) > 1 else ""), ""
    # city: "US, CA, California, Mountain View, 94043, 37.42, -122.08, ..."
    return (parts[0],
            parts[2] if len(parts) > 2 else "",
            parts[3] if len(parts) > 3 else "")


def parse_fields(stdout):
    country = region = city = ""
    for raw in stdout.splitlines():
        line = raw.strip()
        if not line:
            continue
        label, value = "", line
        if ":" in line:
            label, _, value = line.partition(":")
            label = label.strip().lower()
            value = value.strip()
        if label in LABELS:
            # обёртка печатает 'Country:'/'Region:'/'City:' — значение как есть
            if label == "country" and value:
                country = country or value
            elif label == "region" and value:
                region = region or value
            elif label == "city" and value:
                city = city or value
            continue
        kind = edition_kind(label)
        if kind:
            found_country, found_region, found_city = parse_edition(kind, value)
        elif ":" not in line:
            # голая строка без подписи (вид из устаревшей manpage) — это страна
            found_country = line if COUNTRY_LINE_RE.match(line) else ""
            found_region = found_city = ""
        else:
            continue
        country = country or found_country
        region = region or found_region
        city = city or found_city
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
