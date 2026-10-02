#!/usr/bin/env python3
"""git_dumper — поиск оставленного .git на хосте (обёртка над git-dumper).

Вход (stdin JSON): url (цель), wall_timeout (опц., 300).

Вызов: <GIT_DUMPER_BIN|git-dumper> <url> <dump_dir>  (cwd = временная папка).
У arthaud/git-dumper (pip install git-dumper) есть единственная точка входа —
console-script `git-dumper` (setup.cfg: git-dumper = git_dumper:main), а не
`gitdumper` (это другой проект, gitdumper-tool).

Отчёт-файл донор НЕ пишет: весь вывод — построчный лог в stdout через printf
(ошибки — в stderr), плюс обязательный второй позиционный аргумент DIR, куда
складывается дамп. Разбираем stdout построчно:
  [-] Testing <url>/.git/HEAD [<code>]     → {kind: probe}
  [-] Fetching <url>/<path> [<code>]      → {kind: file}
  [-] Already downloaded <url>/<path>     → {kind: file, cached}
  [-] <стадия>                            → {kind: stage}

Доменные ошибки: empty_url, git_dumper_not_installed, timeout (retryable),
tool_failed (сетевая — retryable). Пустой лог = «ничего не нашли»: ok с [].
Платформенные (exit 2): битый JSON входа, не-объект/не-число во входе.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

STAGES = (
    "Fetching .git recursively",
    "Fetching common files",
    "Finding refs/",
    "Finding packs",
    "Finding objects",
    "Fetching objects",
    "Sanitizing .git/config",
    "Running git checkout .",
)

PROBE_RE = re.compile(r"^Testing \S+(?P<target>/\.git/HEAD|/\.git/) \[(?P<code>\d+)\]$")
FETCH_RE = re.compile(r"^Fetching (?P<rest>.+) \[(?P<code>\d+)\]$")
CACHED_RE = re.compile(r"^Already downloaded (?P<rest>\S+)$")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def base_url(url):
    """Нормализация базы — ровно та же, что в fetch_git() донора."""
    u = url.rstrip("/")
    if u.endswith("HEAD"):
        u = u[:-4]
    u = u.rstrip("/")
    if u.endswith(".git"):
        u = u[:-4]
    return u.rstrip("/")


def relative_path(rest, base):
    prefix = base + "/"
    return rest[len(prefix):] if rest.startswith(prefix) else rest


def parse_log(stdout, base):
    findings = []
    for raw in (stdout or "").splitlines():
        line = raw.strip()
        if not line.startswith("[-] "):
            continue
        line = line[4:].strip()
        m = PROBE_RE.match(line)
        if m:
            findings.append({"kind": "probe", "target": m.group("target"),
                             "status": int(m.group("code"))})
            continue
        m = FETCH_RE.match(line)
        if m:
            findings.append({"kind": "file",
                             "path": relative_path(m.group("rest"), base),
                             "status": int(m.group("code")), "cached": False})
            continue
        m = CACHED_RE.match(line)
        if m:
            findings.append({"kind": "file",
                             "path": relative_path(m.group("rest"), base),
                             "status": 0, "cached": True})
            continue
        if line in STAGES:
            findings.append({"kind": "stage", "stage": line})
    return findings


def looks_like_nothing_found(text):
    low = (text or "").lower()
    if "is not a git head file" in low:
        return True
    probed = re.search(r"/\.git/head\s*\[\d+\]", low) or \
        re.search(r"/\.git/\s*\[\d+\]", low)
    return bool(probed) and ("responded with" in low or
                             "zero-length body" in low)


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

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("GIT_DUMPER_BIN", "git-dumper").strip()
    # subprocess поедет с cwd во временной папке: имя из PATH ищем
    # which'ем, путь — приводим к абсолютному
    if "/" not in bin_env and "\\" not in bin_env:
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)

    with tempfile.TemporaryDirectory() as td:
        cmd = [bin_env, url, td]
        if bin_env.lower().endswith(".py"):
            # v0.29: .py-мок запускаем через интерпретатор — прямой exec
            # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
            cmd = [sys.executable] + cmd
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("git_dumper_not_installed",
                        "git-dumper не найден: pip install git-dumper "
                        "(или укажите GIT_DUMPER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"git-dumper не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout или уменьшите объём обхода", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        everything = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if proc.returncode != 0:
            if looks_like_nothing_found(everything):
                return ok({"url": url, "findings": [], "count": 0})
            if "unable to connect" in everything.lower():
                tail = everything.strip().splitlines()
                return fail("tool_failed",
                            f"git-dumper не достучался до цели: "
                            f"{tail[-1] if tail else url}", retryable=True)
            tail = everything.strip().splitlines()
            return fail("tool_failed",
                        f"git-dumper упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")

        findings = parse_log(proc.stdout, base_url(url))

    return ok({"url": url, "findings": findings, "count": len(findings)})


if __name__ == "__main__":
    sys.exit(main())