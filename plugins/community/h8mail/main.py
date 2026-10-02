#!/usr/bin/env python3
"""h8mail — email в утечках (HIBP, Snusbase, Dehashed, Emailrep, IntelX…).

Вход (stdin JSON): target, wall_timeout (опц., общий лимит, 600).

Вызов: <H8MAIL_BIN|h8mail|python3 -m h8mail> -t <target> -c <cfg.ini>
       -o <report.csv> -j <report.json>   (cwd = временная папка).

Ключи API. Инструменту нужен конфиг, поэтому main.py ВСЕГДА пишет его сам во
временный каталог и передаёт флагом -c/--config (флаг подтверждён
README h8mail и h8mail/utils/helpers.py). Формат конфига — ini в секции
[h8mail]: ровно то, что h8mail читает через configparser (его собственный
шаблон `h8mail --gen-config` — такой же). Ключи отсутствуют → пишем пустые
строки, а не пропускаем строки, чтобы состав конфига был предсказуемым.

Отчёт. Основной — CSV отчёта -o: колонки Target,Type,Data (документированы в
wiki h8mail). Если CSV нет, читаем -j: точный вид JSON не документирован, поэ-
му берём из него все пары строк вида [Type, Data]. Ни того, ни другого — no_report;
есть файл, но шапка не Target/Type/Data — bad_report.

Выход (stdout JSON): {target, breaches[{type,data}], checked}. checked — сколько
разных сервисов (префикс Type до подчёркивания) вернули данные. Пустой отчёт —
ok с пустым массивом. Доменные ошибки: empty_target, h8mail_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый
JSON входа, нечисловой wall_timeout, нечитаемый отчёт.
"""
import csv
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 600

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)

# (ключ в секции [h8mail], env-имена по убыванию приоритета) — набор ключей
# взят из шаблона h8mail_config.ini, который генерит `h8mail --gen-config`.
CONFIG_KEYS = (
    ("hibp", ("HIBP_API_KEY",)),
    ("hunterio", ("HUNTERIO_API_KEY",)),
    ("snusbase_token", ("SNUSBASE_TOKEN",)),
    ("weleakinfo_priv", ("WELEAKINFO_PRIV_KEY", "XTLX_API_KEY")),
    ("weleakinfo_pub", ("WELEAKINFO_PUB_KEY",)),
    ("leak-lookup_pub", ("LEAK_LOOKUP_PUB_KEY",)),
    ("leak-lookup_priv", ("LEAK_LOOKUP_PRIV_KEY",)),
    ("emailrep", ("EMAILREP_API_KEY",)),
    ("dehashed_email", ("DEHASHED_EMAIL",)),
    ("dehashed_key", ("DEHASHED_KEY",)),
    ("intelx_key", ("INTELX_KEY",)),
    ("intelx_maxfile", ("INTELX_MAXFILE",)),
    ("breachdirectory_user", ("BREACHDIRECTORY_USER",)),
    ("breachdirectory_pass", ("BREACHDIRECTORY_PASS",)),
)


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("H8MAIL_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("h8mail")
    if found:
        return [found]
    return [sys.executable, "-m", "h8mail"]


def write_config(path):
    """Всегда пишет конфиг: есть env — значение, нет — пустая строка."""
    lines = ["[h8mail]"]
    for key, envs in CONFIG_KEYS:
        value = ""
        for env in envs:
            candidate = os.environ.get(env, "")
            if candidate.strip():
                value = candidate.strip()
                break
        # configparser по умолчанию с BasicInterpolation: «%» надо удвоить,
        # а переводы строк внутри значения сломали бы ini.
        value = value.replace("\n", " ").replace("\r", " ").replace("%", "%%")
        lines.append(f"{key} = {value}")
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n")


def pairs_from_json(node, out):
    if isinstance(node, dict):
        data = node.get("data")
        if isinstance(data, list):
            for item in data:
                if (isinstance(item, (list, tuple)) and len(item) >= 2
                        and isinstance(item[0], str)
                        and isinstance(item[1], str)):
                    out.append((item[0], item[1]))
        for value in node.values():
            pairs_from_json(value, out)
    elif isinstance(node, list):
        for item in node:
            pairs_from_json(item, out)


def from_csv(path):
    with open(path, newline="", encoding="utf-8") as f:
        rows = list(csv.reader(f))
    if not rows:
        return []
    header = [c.strip().lower() for c in rows[0]]
    if "type" not in header and "data" not in header:
        raise ValueError("в CSV h8mail нет колонок Type/Data")
    out = []
    for row in rows[1:]:
        cells = dict(zip(header, [c.strip() for c in row]))
        kind = cells.get("type", "")
        value = cells.get("data", "")
        if not kind and not value:
            continue
        out.append((kind, value))
    return out


def from_json(path):
    with open(path, encoding="utf-8") as f:
        report = json.load(f)
    out = []
    pairs_from_json(report, out)
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

    target = str(data.get("target") or "").strip()
    if not target:
        return fail("empty_target", "target пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        config_path = os.path.join(td, "h8mail_config.ini")
        csv_path = os.path.join(td, "h8mail_report.csv")
        json_path = os.path.join(td, "h8mail_report.json")
        write_config(config_path)
        cmd += ["-t", target, "-c", config_path,
                "-o", csv_path, "-j", json_path]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("h8mail_not_installed",
                        "h8mail не найден: pip install h8mail "
                        "(или укажите H8MAIL_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"h8mail не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        blob = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if proc.returncode != 0:
            if NOT_MODULE_RE.search(blob):
                return fail("h8mail_not_installed",
                            "h8mail не установлен: pip install h8mail "
                            "(или укажите H8MAIL_BIN)")
            tail = blob.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"h8mail упал (exit {proc.returncode}): {last}")

        pairs = []
        if os.path.exists(csv_path):
            try:
                pairs = from_csv(csv_path)
            except (OSError, UnicodeDecodeError, csv.Error, ValueError) as e:
                return fail("bad_report", f"не прочитан CSV h8mail: {e}",
                            exit_code=2)
        elif os.path.exists(json_path):
            try:
                pairs = from_json(json_path)
            except (OSError, UnicodeDecodeError, ValueError) as e:
                return fail("bad_report", f"не прочитан JSON h8mail: {e}",
                            exit_code=2)
        else:
            tail = blob.strip().splitlines()
            last = tail[-1] if tail else "пустой вывод"
            return fail("no_report", f"h8mail не дал отчёта: {last}")

    breaches = []
    sources = set()
    for kind, value in pairs:
        if not kind and not value:
            continue
        breaches.append({"type": kind, "data": value})
        sources.add(kind.split("_", 1)[0] if kind else "unknown")

    return ok({"target": target, "breaches": breaches,
               "checked": len(sources)})


if __name__ == "__main__":
    sys.exit(main())