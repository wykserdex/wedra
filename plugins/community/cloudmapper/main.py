#!/usr/bin/env python3
"""cloudmapper — карта облачной инфраструктуры AWS (обёртка над CloudMapper).

Вход (stdin JSON): account (имя аккаунта cloudmapper), profile (опц., AWS-профиль
для collect), regions (опц., CSV регионов), wall_timeout (опц., лимит на
каждый из двух шагов, 1800).

Вызов (cwd = временная папка):
  <CLOUDMAPPER_BIN|cloudmapper|python3 -m cloudmapper> collect
      --config <cfg.json> --account <account> [--profile P] [--regions CSV]
  <...> prepare --config <cfg.json> --account <account> [--regions CSV]

Флаги подтверждены по исходникам duo-labs/cloudmapper: commands/collect.py
(--config/--account/--profile/--regions), commands/prepare.py (--config/--account/
--regions). Флага --account-name у cloudmapper НЕТ (встречается только в
человеческих шпаргалках); вывода --csv/--json тоже нет — граф всегда пишется в
web/data.json (жёсткий путь в commands/prepare.py).

Конфиг. Обёртка всегда пишет cfg.json сама: collect/prepare требуют аккаунт в
config.json, а его у пользователя обычно нет. Кладём
{"accounts": [{"id": account, "name": account}], "cidrs": {}} — id нужен только
для сборки ARN внутри cloudmapper, значение не валидируется. collect читает
collect_commands.yaml из CWD: обёртка копирует его из каталога установки
донора, если находит (иначе задайте CLOUDMAPPER_BIN на рабочую копию репозитория).

Учётные данные. main.py ключей НЕ читает и не требует: collect сам берёт их из
окружения через boto3 (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN,
AWS_REGION/AWS_DEFAULT_REGION). В тестах инструмент подменён моком.

Отчёт. web/data.json — JSON-массив элементов cytoscape (shared/nodes.py):
узел = {"data": {id, name, type, local_id, parent, node_data}}, ребро =
{"data": {source, target, type: "edge", node_data: [причины]}}.

Выход (stdout JSON): {account, nodes[{id,name,type,parent}], edges[{source,
target,reasons}], count}. count = узлы + рёбра. Пустой граф (нет ресурсов) —
это ok с пустыми массивами, а не ошибка.

Доменные ошибки: empty_account, cloudmapper_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, нечисловой
wall_timeout, нечитаемый web/data.json.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 1800
REPORT_PATH = os.path.join("web", "data.json")

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)
MISSING_ACCOUNT_RE = re.compile(r"Account named .* not found", re.IGNORECASE)


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
    bin_env = os.environ.get("CLOUDMAPPER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("cloudmapper")
    if found:
        return [found]
    return [sys.executable, "-m", "cloudmapper"]


def donor_dirs(cmd):
    dirs = []
    base = os.path.dirname(os.path.abspath(cmd[0]))
    if base:
        dirs.append(base)
        dirs.append(os.path.join(base, "cloudmapper"))
    try:
        import importlib.util
        spec = importlib.util.find_spec("cloudmapper")
        if spec is not None and spec.origin:
            dirs.append(os.path.dirname(os.path.abspath(spec.origin)))
    except Exception:
        pass
    out = []
    for path in dirs:
        if path and path not in out:
            out.append(path)
    return out


def stage_donor_files(cmd, td):
    """collect читает collect_commands.yaml из CWD — копируем из установки."""
    for base in donor_dirs(cmd):
        source = os.path.join(base, "collect_commands.yaml")
        if os.path.isfile(source):
            shutil.copyfile(source, os.path.join(td, "collect_commands.yaml"))
            return True
    return False


def write_config(td, account):
    config = {"accounts": [{"id": account, "name": account}], "cidrs": {}}
    path = os.path.join(td, "config.json")
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(config, f)
    return path


def run_step(cmd, td, wall, label):
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True,
                              cwd=td, timeout=wall)
    except FileNotFoundError:
        return None, "cloudmapper_not_installed", (
            "cloudmapper не найден: поставьте из репозитория "
            "https://github.com/duo-labs/cloudmapper (pip install -r "
            "requirements.txt) или укажите CLOUDMAPPER_BIN")
    except subprocess.TimeoutExpired:
        return None, "timeout", (
            f"{label} не уложился в {wall:.0f}s: сузьте regions или "
            "увеличьте wall_timeout")
    if proc.stdout:
        sys.stderr.write(proc.stdout)
    if proc.stderr:
        sys.stderr.write(proc.stderr)
    return proc, None, None


def classify(proc, label):
    blob = (proc.stdout or "") + "\n" + (proc.stderr or "")
    if proc.returncode == 0:
        return None, None
    if NOT_MODULE_RE.search(blob):
        return "cloudmapper_not_installed", (
            "cloudmapper не установлен: поставьте из репозитория "
            "duo-labs/cloudmapper или укажите CLOUDMAPPER_BIN")
    tail = blob.strip().splitlines()
    last = tail[-1] if tail else f"exit {proc.returncode}"
    if MISSING_ACCOUNT_RE.search(blob):
        return "bad_account", (
            f"cloudmapper не знает аккаунт: {last}")
    return "tool_failed", f"{label} упал (exit {proc.returncode}): {last}"


def parse_graph(payload):
    if not isinstance(payload, list):
        raise ValueError("web/data.json — не массив")
    nodes = []
    edges = []
    for item in payload:
        if not isinstance(item, dict):
            continue
        data = item.get("data")
        if not isinstance(data, dict):
            continue
        if data.get("source") and data.get("target"):
            reasons = data.get("node_data")
            edges.append({
                "source": str(data["source"]),
                "target": str(data["target"]),
                "reasons": len(reasons) if isinstance(reasons, list) else 0,
            })
            continue
        if not data.get("id"):
            continue
        nodes.append({
            "id": str(data.get("id") or ""),
            "name": str(data.get("name") or ""),
            "type": str(data.get("type") or ""),
            "parent": str(data.get("parent") or ""),
        })
    return nodes, edges


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

    account = str(data.get("account") or "").strip()
    if not account:
        return fail("empty_account", "account пуст")

    profile = str(data.get("profile") or "").strip()
    regions = str(data.get("regions") or "").strip()

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        config_path = write_config(td, account)
        stage_donor_files(cmd, td)

        collect_cmd = cmd + ["collect", "--config", config_path,
                             "--account", account]
        if profile:
            collect_cmd += ["--profile", profile]
        if regions:
            collect_cmd += ["--regions", regions]
        proc, code, message = run_step(collect_cmd, td, wall, "cloudmapper collect")
        if code:
            return fail(code, message, retryable=(code == "timeout"))

        prepare_cmd = cmd + ["prepare", "--config", config_path,
                             "--account", account]
        if regions:
            prepare_cmd += ["--regions", regions]
        proc, code, message = run_step(prepare_cmd, td, wall, "cloudmapper prepare")
        if code:
            return fail(code, message, retryable=(code == "timeout"))

        code, message = classify(proc, "cloudmapper prepare")
        if code:
            return fail(code, message, retryable=(code == "timeout"))

        report_path = os.path.join(td, REPORT_PATH)
        if not os.path.exists(report_path):
            tail = ((proc.stderr or "") + (proc.stdout or "")).strip().splitlines()
            last = tail[-1] if tail else "пустой вывод"
            return fail("no_report", f"cloudmapper не дал {REPORT_PATH}: {last}")
        try:
            with open(report_path, encoding="utf-8") as f:
                payload = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан {REPORT_PATH}: {e}",
                        exit_code=2)

    try:
        nodes, edges = parse_graph(payload)
    except ValueError as e:
        return fail("bad_report", f"неожиданный формат {REPORT_PATH}: {e}",
                    exit_code=2)

    return ok({"account": account, "nodes": nodes, "edges": edges,
               "count": len(nodes) + len(edges)})


if __name__ == "__main__":
    sys.exit(main())