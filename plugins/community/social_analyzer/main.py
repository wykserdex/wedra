#!/usr/bin/env python3
"""social_analyzer — username → профили по 400+ соцсетям (обёртка над CLI).

Вход (stdin JSON): username, websites[] (опц., --websites), wall_timeout
(опц., общий лимит, 600).

Вызов: <SOCIAL_ANALYZER_BIN|python3 -m social-analyzer> --username <u>
       [--websites "a b c"] --output json   (cwd = временная папка).

Неоднозначность пакета закрыта так: имя модуля с дефисом (`-m social-analyzer`)
и консольный скрипт `social-analyzer` — один и тот же CLI по докам QeeqBox;
берём `-m`, он не зависит от того, попал ли Scripts/ в PATH.

Разбор отчёта (паттерн B, stdout). Сначала пробуем машинный JSON из --output
json: обходим структуру рекурсивно и собираем любые объекты, у которых есть
поле со ссылкой (link/url/uri/profile). Точный вид ключей social-analyzer
меняется между версиями, поэтому берём «что похоже на профиль», а не
жёсткую схему. Если JSON не распознан (старый релиз / обрезанный вывод) —
fallback на текст: строка с URL, помеченная «Found»/«[+]», идёт в found,
любая другая строка с URL — в checked.

Выход (stdout JSON): {username, found[{site,url}], checked}. checked — сколько
профилей всего разобрано (найденные + ненайденные + неудачные), site — имя
сайта из JSON либо host из ссылки. Пустой результат — ok с found=[]. Доменные
ошибки: empty_username, social_analyzer_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, websites не
массив, нечисловой wall_timeout.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import urlparse

DEFAULT_WALL = 600

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
URL_RE = re.compile(r"https?://[^\s\"'<>,;)\]]+")
LINK_KEYS = ("link", "url", "uri", "profile", "profile_url")
NAME_KEYS = ("name", "website", "site", "platform", "source")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("SOCIAL_ANALYZER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    return [sys.executable, "-m", "social-analyzer"]


def site_of(name, url):
    label = str(name or "").strip()
    if not label:
        try:
            label = (urlparse(url).netloc or "").lower()
        except ValueError:
            label = ""
        if label.startswith("www."):
            label = label[4:]
    return label


def clean_url(raw):
    return raw.rstrip(".,;:!?)").strip()


def hint_for(key, hint):
    """Имя секции верхнего уровня — подсказка «найдено / не найдено»."""
    low = str(key).lower()
    if "detect" in low or "found" in low or "hit" in low:
        return "yes"
    if any(w in low for w in ("unknown", "fail", "bad", "error", "miss")):
        return "no"
    return hint


def is_detected(node, hint):
    status = str(node.get("status") or node.get("state") or "").strip().lower()
    if status:
        if "detect" in status or status in ("found", "true", "yes", "claimed"):
            return True
        return False
    if isinstance(node.get("exists"), bool):
        return node["exists"]
    rate = node.get("rate")
    if isinstance(rate, (int, float)) and not isinstance(rate, bool):
        return rate > 0
    return hint == "yes"


def walk(node, out, hint="yes"):
    """Рекурсивный обход JSON: всё, что похоже на профиль, — в out."""
    if isinstance(node, dict):
        url = ""
        for key in LINK_KEYS:
            value = node.get(key)
            if isinstance(value, str) and value.strip().lower().startswith("http"):
                url = clean_url(value)
                break
        if url:
            name = ""
            for key in NAME_KEYS:
                value = node.get(key)
                if isinstance(value, str) and value.strip():
                    name = value.strip()
                    break
            out.append({"site": site_of(name, url), "url": url,
                        "detected": is_detected(node, hint)})
        for key, value in node.items():
            walk(value, out, hint_for(key, hint))
    elif isinstance(node, list):
        for item in node:
            walk(item, out, hint)


def parse_json_report(text):
    stripped = text.strip()
    if not stripped or stripped[0] not in ("[", "{"):
        return None
    for start in range(len(stripped)):
        if stripped[start] not in "[{":
            continue
        try:
            report = json.loads(stripped[start:])
            return report
        except ValueError:
            break
    return None


NEG_RE = re.compile(r"\b(not\s*found|unknown|failed|no\s*result|missing)\b",
                    re.IGNORECASE)
POS_RE = re.compile(r"\bfound\b|\[\+\]", re.IGNORECASE)


def parse_text_report(text):
    found = []
    checked = []
    for raw in text.splitlines():
        line = ANSI_RE.sub("", raw)
        hit = URL_RE.search(line)
        if not hit:
            continue
        url = clean_url(hit.group(0))
        detected = bool(POS_RE.search(line)) and not NEG_RE.search(line)
        record = {"site": site_of("", url), "url": url}
        if detected:
            found.append(record)
        else:
            checked.append(record)
    return found, checked


def dedup(records):
    out = []
    seen = set()
    for rec in records:
        if rec["url"] in seen:
            continue
        seen.add(rec["url"])
        out.append({"site": rec["site"], "url": rec["url"]})
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

    username = str(data.get("username") or "").strip()
    if not username:
        return fail("empty_username", "username пуст")

    websites = data.get("websites")
    if websites is not None and not isinstance(websites, list):
        return fail("bad_websites", "websites обязан быть массивом", exit_code=2)
    if isinstance(websites, list):
        websites = [str(w).strip() for w in websites if str(w).strip()]

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += ["--username", username]
    if websites:
        cmd += ["--websites", " ".join(websites)]
    cmd += ["--output", "json"]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("social_analyzer_not_installed",
                        "social-analyzer не найден: pip install "
                        "social-analyzer (или укажите SOCIAL_ANALYZER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"social-analyzer не уложился в {wall:.0f}s: уменьшите "
                        "websites или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

    if proc.returncode != 0:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()
        last = tail[-1] if tail else f"exit {proc.returncode}"
        return fail("tool_failed",
                    f"social-analyzer упал (exit {proc.returncode}): {last}")

    stdout = proc.stdout or ""
    if not stdout.strip() and not (proc.stderr or "").strip():
        return fail("no_report", "social-analyzer не дал вывода")

    report = parse_json_report(stdout)
    if report is not None:
        profiles = []
        walk(report, profiles)
        found = dedup([p for p in profiles if p["detected"] and p["site"]])
        return ok({"username": username, "found": found,
                   "checked": len(profiles)})

    hit_found, hit_checked = parse_text_report(stdout)
    if not hit_found and not hit_checked:
        tail = stdout.strip().splitlines()
        last = tail[-1] if tail else "пустой stdout"
        return fail("no_report",
                    f"social-analyzer не дал распознаваемый репорт: {last}")
    found = dedup(hit_found)
    return ok({"username": username, "found": found,
               "checked": len(hit_found) + len(hit_checked)})


if __name__ == "__main__":
    sys.exit(main())