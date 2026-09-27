![WEDRA](banner.png)

[English](README.en.md) | Русский

# WEDRA — локальный оркестратор цепочек с человеком в петле

Пишешь YAML — WEDRA запускает плагины отдельными процессами, проверяет их контракт,
показывает решения человеку и пишет журнал каждого шага. Всё локально, один бинарник.

- **Человек в петле** — важные шаги ждут твоего `accept`, а не исполняются молча
- **Контракты вместо доверия** — плагин получает только объявленные входы, выход проверяется
- **MCP из коробки** — LLM-агент собирает и запускает пайплайны, подтверждает человек
- **Офлайн по умолчанию** — unset `network` значит `deny`, журнал `journal.jsonl`, доказуемый resume
- **Чужой код — в песочницу** — untrusted-плагины на Linux идут в `bwrap` с отдельным netns,
  на macOS/Windows — отказ запуска (fail-closed), а не тихий запуск без изоляции

## Быстрый старт за 2 минуты (без сборки)

```bash
# 1. Скачай бинарник (Linux; для macOS/Windows — тот же путь в Releases)
curl -L -o wedra https://github.com/wykserdex/wedra/releases/latest/download/wedra-linux-amd64
chmod +x wedra

# 2. Возьми примеры и плагины
git clone --depth 1 https://github.com/wykserdex/wedra
cd wedra

# 3. Запусти офлайн-демо: текст → метрики → человек подтверждает
./wedra pipeline run examples/text_stats.yaml --yes
```

Ожидаемый итог:

```text
→ stats        (попытка 1/1)

══ human_gate · review ══
    steps.stats.lines = 3
    steps.stats.words = 18
    steps.stats.unique_words = 16
    steps.stats.longest_word = "Оркестратор"
  [--yes] auto-accept

■ ран завершён: ok=1 aborted=0
```

Без `--yes` гейт спросит решение интерактивно: `a` — принять, `r` — отклонить.
`--yes` не обходит `approval: human` и `pipeline.gates: human_only`.

Требования: Python 3.9+ в `PATH` для Python-плагинов. Go 1.22+ нужен только для сборки
из исходников. Для untrusted-плагинов на Linux нужен `bwrap`, а для их сетевого доступа —
ещё и `slirp4netns`, иначе ран будет отклонён, а не запущен без изоляции.

## Минимальный pipeline

```yaml
format_version: "0.2"
pipeline:
  name: example
  input:
    text: "hello world hello"
  network: deny  # явное решение; unset — тоже deny, но здесь видно намерение
  steps:
    - id: stats
      plugin: plugins/community/text_analyzer
      on_error: stop
    - id: review
      plugin: core/human_gate
      form:
        - { field: steps.stats.words, editable: false }
        - { field: steps.stats.longest_word, editable: false }
      actions: [accept, reject]
      on_reject: stop
```

```bash
./wedra pipeline validate hello.yaml  # статическая проверка до запуска
./wedra pipeline plan hello.yaml      # граф шагов и зависимостей
./wedra pipeline run hello.yaml       # запуск с гейтом человека
```

`core/human_gate` — единственный встроенный plugin; другие `core/*` отклоняются.
Полный формат и коды ошибок — в [protocol/v0.2/PROTOCOL.md](protocol/v0.2/PROTOCOL.md)
и [protocol/v0.2/ERRORS.md](protocol/v0.2/ERRORS.md).

## Что умеет

| Возможность | Как выглядит |
|---|---|
| Условия | `when: { path: steps.stats.words, op: gte, value: 10 }` — шаг пропускается |
| Циклы | `foreach` по массиву на уровне пайплайна или отдельного шага |
| Параллельность | `parallel_group` — ветки идут одновременно, дальше барьер |
| Повторы | `retry` и доказуемый `resume` прерванного рана |
| Проверки | `validate`/`lint` с кодами ошибок и machine-readable `--json` |
| Плагины | отдельный процесс, JSON по stdin/stdout, `plugin create/test/install` |
| Агенты (MCP) | 7 инструментов: `list_plugins`, `describe_plugin`, `validate_pipeline`, `plan_pipeline`, `run_pipeline`, `get_run`, `cancel_run` |
| Контроль | GUI локально, HTTP API, журнал `journal.jsonl`, снапшоты контекста |

Примеры под разные вкусы: `examples/text_stats.yaml` (старт),
`examples/when_demo.yaml`, `examples/parallel_demo.yaml`,
`examples/quick_intel.yaml` (офлайн-разбор цели: домен, IP, email, хэш, гео),
`examples/email_triage_chain.yaml` (проверка email без LLM),
`examples/llm_text_chain.yaml` (LLM-цепочка: Gemini-черновик → человек → OpenAI-правка).

## Чем отличается

| | WEDRA | n8n | Temporal / Windmill |
|---|---|---|---|
| Запуск | один бинарник локально | сервер / Docker | кластер + SDK / сервер |
| Гейт человека в ране | встроен: accept/reject + resume | собирается нодами | пишется кодом |
| Контракт шага | строгий: типы, форматы, permissions | свободный JS/Python | свободный код |
| MCP для агентов | встроенный stdio-сервер; у агента нет кнопки approve | есть (MCP Server Trigger, SSE/HTTP) | есть (интеграции) |
| Изоляция чужого кода | `bwrap` на Linux, fail-closed на macOS/Windows | зависит от деплоя | зависит от деплоя |
| Зрелость и границы | честная 0.x: локальные цепочки, лимиты в SECURITY.md | зрелая экосистема, сотни интеграций | энтерпрайз-масштаб |

## MCP: агент предлагает — человек подтверждает

```bash
./wedra mcp --plugins "$PWD/plugins" --workdir "$PWD" --no-gui
```

Агент видит 7 инструментов: от `list_plugins` до `run_pipeline` и `cancel_run`.
Кнопки «принять» у агента нет — `gate_decision` существует только как событие журнала,
а не как инструмент: на `waiting_human` человек открывает GUI и решает в браузере.
Без GUI гейт можно закрыть командой `wedra approve <run_id> <step_id>` — она требует
интерактивный TTY и подтверждение введённым с клавиатуры кодом.

MCP ограничивает plugin refs и `file_ref` рабочей директорией, отклоняет `any_host`,
loopback/private hosts и symlink escape. Это policy boundary, а не замена изоляции ОС.
Подробности: [docs/mcp.md](docs/mcp.md).

## Плагины и реестр

Плагин — отдельный процесс: читает JSON из stdin, пишет JSON-конверт в stdout,
объявляет манифест с типами входов/выходов и разрешениями:

```bash
./wedra plugin create ./my_plugin --example array  # скелет, сразу зелёный
./wedra plugin validate ./my_plugin
./wedra plugin test ./my_plugin
./wedra plugin install text_analyzer               # из реестра
./wedra pipeline install email_check               # пресет + его плагины
./wedra registry validate --registry=registry.yaml
```

`permissions` — декларация и аудит, а не OS-песочница. Секреты передаются только именами
env-переменных, объявленными в манифесте и пайплайне. Если плагин объявляет
`runtime.requires`, используются exact pins `package==version`, рядом обязателен
`requirements.lock`.

Удалённые источники ставятся только с полным SHA-пином коммита (40/64-hex). Пин должен быть
**предком текущего HEAD** (`git merge-base --is-ancestor`): checkout на более позднем коммите,
содержащем пин, допускается, на несовместимой ветке — отказ установки.

Прямой URL пресета — тоже недоверенный вход: принимается только `https://`, редирект обязан
остаться в той же схеме и на том же host. Рядом пишется sidecar `<name>.yaml.sha256`, формат
совместим с `sha256sum -c`.

Авторский гайд: [docs/plugin-dev.md](docs/plugin-dev.md).

## Секреты и сеть

Ключи в YAML не пишутся — только имена. Имя должно совпадать с тем, что плагин просит
в `permissions.secrets` (у `llm_openai` это `LLM_OAI_API_KEY`, не `OPENAI_API_KEY`):

```yaml
pipeline:
  secrets: [LLM_OAI_API_KEY]
```

Сабпроцесс получает allowlist-окружение и только объявленные секреты. `network: deny`
и незаполненное поле означают одно и то же, поэтому шаги с сетевыми permissions требуют
явного `network: allow`, и решение попадает в журнал. Перечислить конкретные хосты нельзя:
точечный фильтр не реализован, поэтому `allow` вместе со списком `host:port` отвергается
(`E_NETWORK_NOT_ENFORCEABLE`) — нужен `any_host: true`. Плагин с `any_host` получает egress
через отдельный userspace-стек, а не хостовую сеть.

## CLI

```text
wedra pipeline validate <file.yaml> [--json]
wedra pipeline lint <file.yaml>
wedra pipeline plan <file.yaml>
wedra pipeline run <file.yaml> [--yes] [--resume=<run_id>] [--deny-untrusted-plugins] [--allow-untrusted-plugins]
wedra runs list | runs show <run_id> | runs resume <run_id> <file.yaml> [--yes]
wedra approve <run_id> <step_id>                      # закрыть гейт без GUI (TTY + код подтверждения)
wedra plugin create|validate|test|install|inspect|list|search
wedra pipeline install <name|file.yaml|url>
wedra registry validate --registry=registry.yaml
wedra gui [--port 8765] [--open] [--plugins=<dir>] [--pipelines=<dir>]
wedra mcp --plugins <dir> --workdir <dir> [--no-gui]
```

`--deny-untrusted-plugins` запрещает любой untrusted-плагин в ранде (рекомендуется для CI),
`--allow-untrusted-plugins` разрешает внешний код — он уходит в изолятор, и без рабочего
изолятора на хосте запуск падает, а не выполняется без него.

`tool` сохранён как compatibility-поверхность, новые сценарии — в `wedra`.

## Безопасность

Модель угроз описана в [SECURITY.md](SECURITY.md), коротко:

- доверенный локальный оператор и заранее проверенные плагины — штатный режим;
- `sandbox: untrusted` в манифесте + явный флаг оператора (`--allow-untrusted-plugins`)
  — запуск только в OS-песочнице;
- Linux: `bwrap` (read-only ФС, отдельные PID/IPC/UTS/net namespaces, egress через
  `slirp4netns`); macOS/Windows: backend нет — untrusted отклоняется, а не запускается;
- это сильная, но не полная граница для враждебного кода: фильтра по destination нет,
  чтение файлов пользователя не ограничивается;
- не передавайте секреты через URL, pipeline input или метаданные реестра.

## Структура репозитория

```text
cmd/wedra/            primary CLI
cmd/wedragui/         desktop launcher
cmd/tool/             compatibility CLI
internal/pipeline/    model, parser, validation, planning
internal/execution/   runner, resume, control flow
internal/plugin/      manifest, subprocess, contract
internal/registry/    registry, install, commit pins
internal/journal/     append-only journal and stores
internal/gate/        human gate
internal/api/         HTTP/GUI adapter
internal/mcp/         MCP adapter
plugins/              official and community plugins
examples/             canonical pipelines and presets
conformance/          public conformance corpus
schemas/              pipeline and manifest schemas
docs/                 architecture, quickstart, plugin authoring
var/runs/             runtime output; ignored by git
```

`internal/core` и `cmd/tool` — transitional compatibility layers. Не переименовывайте
канонические директории без отдельного RFC/ADR и migration plan; структура описана
в [docs/architecture.md](docs/architecture.md).

## Релизы

Стабильные сборки — в [GitHub Releases](https://github.com/wykserdex/wedra/releases):
`wedra-{linux,darwin,windows}-{amd64,arm64}`, legacy `tool-*`, `wedragui-windows-*.exe`,
conformance-пакет, `SHA256SUMS`, SBOM (`sbom.spdx.json`) и build provenance.
Тег релиза совпадает с [`VERSION`](VERSION) — сейчас `0.32c`;
protocol version — [`protocol/VERSION`](protocol/VERSION) (`0.2`), версии независимы.

## Документация

- [Quickstart: тест за 5 минут](docs/quickstart.md)
- [Architecture](docs/architecture.md) · [MCP](docs/mcp.md) · [Plugin development](docs/plugin-dev.md)
- [Формат пайплайна и коды ошибок](protocol/v0.2/PROTOCOL.md) · [Versioning](docs/versioning.md)
- [Governance](GOVERNANCE.md) · [Contributing](CONTRIBUTING.md) · [Changelog](CHANGELOG.md) · [Security](SECURITY.md)

## Статус

Активная разработка, честная 0.x. Нашёл баг или хочешь плагин в реестр — открывай issue.
