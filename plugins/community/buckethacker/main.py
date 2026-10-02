#!/usr/bin/env python3
"""buckethacker — поиск открытых облачных хранилищ по имени (обёртка над CLI).

Вход (stdin JSON): name (имя бакета/контейнера/storage-аккаунта), wall_timeout
(опц., общий лимит, 300).

Вызов: <BUCKETHACKER_BIN|buckethacker|python3 -m buckethacker> <name>
       (cwd = временная папка). Других флагов НЕТ намеренно: CLI донора в сети
       не подтверждён (репозиторий ingo60/buckethacker не существует), поэтому
       не выдумываем опции — только позиционный аргумент, как у поисковиков
       имён хранилищ.

Отчёт. Отчёт-файла у инструмента не ожидаем: разбираем stdout.
  1) если stdout — JSON (целиком или встроенный в текст блок {...}/[...]),
     берём список записей и читаем ключи provider|service|type|cloud,
     services|services_found, open|is_open|isOpen|public|found|status;
  2) иначе — осторожный разбор текста построчно: строка относится к сервису,
     если в ней встречается его маркер (s3/amazon, azure/blob, gcp/google,
     digitalocean/spaces, alibaba/oss, backblaze/b2, cloudflare/r2); строка
     считается открытой, если в ней есть open|public|found|exists|accessible
     и нет closed|private|forbidden|denied|not found|no such. Числа в тексте
     (403/404) трактуются как «не открыто» и вырезаются из маркеров.

Выход (stdout JSON): {name, provider, open, services, count}. provider —
провайдер первой записи, где хранилище открыто (иначе первый распознанный, иначе
"" ); open — открыто ли хоть где; services — упорядоченный список сервисов
(в текстовом режиме — нормализованные имена: aws|azure|gcp|digitalocean|
alibaba|backblaze|cloudflare, в JSON — строки как их отдал инструмент);
count = len(services). Ничего не нашлось — ok с пустым массивом и open=false.

Доменные ошибки: empty_name, buckethacker_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, нечисловой
wall_timeout, нечитаемый отчёт.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)

# (нормализованный провайдер, маркеры в строке отчёта)
PROVIDER_HINTS = (
    ("aws", ("s3", "amazon")),
    ("azure", ("azure", "blob")),
    ("gcp", ("gcp", "google", "storage.googleapis")),
    ("digitalocean", ("digitalocean", "spaces")),
    ("alibaba", ("alibaba", "oss")),
    ("backblaze", ("backblaze", "b2")),
    ("cloudflare", ("cloudflare", "r2")),
)

OPEN_MARKERS = ("open", "public", "found", "exists", "accessible", "allowed")
CLOSED_MARKERS = ("closed", "private", "forbidden", "denied", "not found",
                  "no such", "missing", "error", "unknown")
HTTP_CODES = ("200", "204", "206", "301", "302", "400", "401", "403", "404",
              "405", "500", "503")

PROVIDER_KEYS = ("provider", "service", "type", "cloud", "platform")
SERVICES_KEYS = ("services", "services_found", "found_services", "providers",
                 "targets")
OPEN_KEYS = ("open", "is_open", "isOpen", "public", "found", "accessible",
             "exposed", "is_public")

TRUE_WORDS = ("true", "yes", "y", "1", "open", "public", "found", "exists",
              "accessible", "allowed", "ok", "success")


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
    bin_env = os.environ.get("BUCKETHACKER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("buckethacker")
    if found:
        return [found]
    return [sys.executable, "-m", "buckethacker"]


def truthy(value):
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        return value.strip().lower() in TRUE_WORDS
    return False


def record_open(record):
    for key in OPEN_KEYS:
        if key in record and truthy(record[key]):
            return True
    for key in ("status", "result", "state", "access"):
        if key in record:
            text = str(record[key]).strip().lower()
            if text in TRUE_WORDS:
                return True
            if any(bad in text for bad in CLOSED_MARKERS):
                return False
    return False


def record_provider(record):
    for key in PROVIDER_KEYS:
        value = record.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def flatten_services(value):
    out = []
    if isinstance(value, str):
        if value.strip():
            out.append(value.strip())
    elif isinstance(value, dict):
        for key in PROVIDER_KEYS:
            if isinstance(value.get(key), str) and value[key].strip():
                out.append(value[key].strip())
                break
        else:
            if str(value.get("name") or "").strip():
                out.append(str(value["name"]).strip())
    elif isinstance(value, list):
        for item in value:
            out.extend(flatten_services(item))
    return out


def record_services(record):
    services = []
    for key in SERVICES_KEYS:
        if key in record:
            services.extend(flatten_services(record[key]))
    provider = record_provider(record)
    if provider:
        services.insert(0, provider)
    return services


def loads(text):
    try:
        return json.loads(text), False
    except ValueError:
        return None, True


def extract_json(text):
    stripped = text.strip()
    if not stripped:
        return None
    if stripped[0] in "{[":
        value, broken = loads(stripped)
        return "BROKEN" if broken else value
    starts = [i for i in (stripped.find("{"), stripped.find("[")) if i >= 0]
    if not starts:
        return None
    start = min(starts)
    for opener, closer in (("{", "}"), ("[", "]")):
        end = stripped.rfind(closer)
        if end <= start or stripped[start] != opener:
            continue
        value, broken = loads(stripped[start:end + 1])
        if not broken:
            return value
    return None


def from_records(payload):
    if isinstance(payload, dict):
        records = payload.get("results")
        if not isinstance(records, list):
            records = payload.get("buckets")
        if not isinstance(records, list):
            records = [payload]
    elif isinstance(payload, list):
        records = payload
    else:
        return None
    services = []
    provider = ""
    first_provider = ""
    is_open = False
    for record in records:
        if not isinstance(record, dict):
            text = str(record).strip()
            if text:
                services.append(text)
            continue
        found_provider = record_provider(record)
        if found_provider and not first_provider:
            first_provider = found_provider
        if record_open(record):
            is_open = True
            if not provider:
                provider = found_provider
        services.extend(record_services(record))
    if not provider:
        provider = first_provider
    return provider, is_open, services


def detect_provider(line):
    low = line.lower()
    for name, markers in PROVIDER_HINTS:
        for marker in markers:
            if re.search(r"(?<![a-z0-9])" + re.escape(marker) + r"(?![a-z0-9])",
                         low):
                return name
    return ""


def strip_codes(line):
    for code in HTTP_CODES:
        line = line.replace(code, " ")
    return line


def from_text(text):
    services = []
    provider = ""
    first_provider = ""
    is_open = False
    for raw in text.splitlines():
        line = strip_codes(raw.strip())
        if not line:
            continue
        name = detect_provider(line)
        if not name:
            continue
        low = line.lower()
        if not first_provider:
            first_provider = name
        services.append(name)
        if any(bad in low for bad in CLOSED_MARKERS):
            continue
        if any(good in low for good in OPEN_MARKERS):
            is_open = True
            if not provider:
                provider = name
    if not provider:
        provider = first_provider
    return provider, is_open, services


def dedup(values):
    out = []
    for value in values:
        if value and value not in out:
            out.append(value)
    return out


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

    name = str(data.get("name") or "").strip()
    if not name:
        return fail("empty_name", "name пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd.append(name)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("buckethacker_not_installed",
                        "buckethacker не найден: pip install buckethacker "
                        "(или укажите BUCKETHACKER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"buckethacker не уложился в {wall:.0f}s: "
                        "уменьшите нагрузку или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    blob = proc.stdout or ""
    if proc.returncode != 0:
        combined = blob + "\n" + (proc.stderr or "")
        if NOT_MODULE_RE.search(combined):
            return fail("buckethacker_not_installed",
                        "buckethacker не установлен: pip install buckethacker "
                        "(или укажите BUCKETHACKER_BIN)")
        tail = (proc.stderr or blob).strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"buckethacker упал (exit {proc.returncode}): {last}")

    if not blob.strip():
        return fail("no_report", "buckethacker не дал вывода на stdout")

    payload = extract_json(blob)
    if payload == "BROKEN":
        return fail("bad_report", "на stdout начало JSON, но он не разбирается",
                    exit_code=2)
    if payload is not None:
        parsed = from_records(payload)
        if parsed is None:
            return fail("bad_report", "неожиданный формат вывода buckethacker",
                        exit_code=2)
        provider, is_open, services = parsed
    else:
        provider, is_open, services = from_text(blob)

    services = dedup(services)
    if not provider and services:
        provider = services[0]

    return ok({"name": name, "provider": provider, "open": bool(is_open),
               "services": services, "count": len(services)})


if __name__ == "__main__":
    sys.exit(main())