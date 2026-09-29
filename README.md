![WEDRA](banner.png)

[English](README.en.md) | Русский

# WEDRA — локальный оркестратор цепочек с человеком в петле

Пишешь YAML — WEDRA запускает плагины отдельными процессами, проверяет их контракт,
показывает решения человеку и пишет журнал каждого шага. Всё локально, один бинарник.

- **Человек в петле** — важные шаги ждут твоего `accept`, а не исполняются молча
- **Контракты вместо доверия** — плагин получает только объявленные входы, выход проверяется
- **MCP из коробки** — LLM-агент собирает и запускает пайплайны, а опасные шаги
  не запускаются без `human_gate` (иначе `E_GATE_REQUIRED`)
- **Офлайн по умолчанию** — unset `network` значит `deny`, журнал `journal.jsonl`, доказуемый resume
- **Чужой код — в песочницу** — untrusted-плагины на Linux идут в `bwrap` с отдельным netns,
  на macOS/Windows — отказ запуска (fail-closed), а не тихий запуск без изоляции

## Быстрый старт за 2 минуты (без сборки)

Скачай бинарник — и всё. Дальше `wedra demo`: цепочка целиком из встроенных
модулей, поэтому не нужны ни git, ни Python, ни сеть.

```bash
curl -L -o wedra https://github.com/wykserdex/wedra/releases/latest/download/wedra-linux-amd64
chmod +x wedra
./wedra demo
```

Ожидаемый итог:

```text
▶ запуск "demo"  (журнал: /tmp/wedra-demo-…/20260927-…-demo-…)

══ human_gate · review ══
    steps.stats.lines = 3
    steps.stats.words = 11
    steps.stats.unique_words = 11
    steps.stats.longest_word = "Оркестратор"
  [--yes] auto-accept

■ ран завершён: ok=1 aborted=0
```

Это тот же ран, что `pipeline run`, с теми же журналом, гейтом и `resume` —
просто собранный из встроенного `core/text_stats` и встроенного гейта, без
внешних плагинов.

Когда захочется настоящих плагинов и примеров — клонируй репозиторий:

```bash
git clone --depth 1 https://github.com/wykserdex/wedra
cd wedra
./wedra pipeline run examples/text_stats.yaml --yes
```

Требования: Python 3.9+ в `PATH` — только для Python-плагинов (встроенным и
`wedra demo` он не нужен). Go 1.22+ нужен только для сборки из исходников. Для
untrusted-плагинов на Linux нужен `bwrap`, а для их сетевого доступа — ещё и
`slirp4netns`, иначе ран будет отклонён, а не запущен без изоляции.

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

Встроенные модули: `core/human_gate` (шлюз) и `core/text_stats` (метрики
текста). Namespace `core/` закрыт — любой другой `core/*` отвергается, а не
ищется на диске. Полный формат, список встроенных и коды ошибок — в
[protocol/v0.2/PROTOCOL.md](protocol/v0.2/PROTOCOL.md) и
[protocol/v0.2/ERRORS.md](protocol/v0.2/ERRORS.md).

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
| MCP для агентов | встроенный stdio-сервер; у агента нет кнопки approve, а опасный шаг без гейта отклоняется | есть (MCP Server Trigger, SSE/HTTP) | есть (интеграции) |
| Изоляция чужого кода | `bwrap` на Linux, fail-closed на macOS/Windows | зависит от деплоя | зависит от деплоя |
| Зрелость и границы | честная 0.x: локальные цепочки, лимиты в SECURITY.md | зрелая экосистема, сотни интеграций | энтерпрайз-масштаб |

## MCP: агент предлагает — человек подтверждает опасное

```bash
./wedra mcp --plugins "$PWD/plugins" --workdir "$PWD" --no-gui
```

Агент видит 8 инструментов: от `list_plugins` до `run_pipeline`, `cancel_run` и
`exec_plugin`. Кнопки «принять» у агента нет — `gate_decision` существует только как
событие журнала, а не как инструмент: на `waiting_human` человек открывает GUI и
решает в браузере. Второе, не менее важное: агент не может выкинуть гейт, чтобы
одобрение не понадобилось. `run_pipeline` отказывает с `E_GATE_REQUIRED`, если
первый шаг с capabilities «сеть / запись на диск / чтение секретов» (по
`permissions` манифеста) идёт без `core/human_gate` перед собой. Гейт **после**
опасного шага не считается: человек увидел бы результат, а не намерение. Шаги
без таких прав (посчитать слова, разобрать CSV) идут без гейта — иначе правило
запрещало бы агенту полезную работу.

Обход есть, но он операторский и не тихий: `--allow-unapproved-runs` печатает
предупреждение при старте и пишет каждый обойдённый ран в
`<runs-dir>/gate-bypass.jsonl`; если след не записался — отказ до старта.
По умолчанию обхода нет.

Про `wedra approve`: команда **не закрывает гейт**. Она требует TTY и код,
подтверждающий намерение человека, после чего показывает защищённый API-вызов
`POST /api/runs/<id>/gate`. Окончательное решение отправляет человек из GUI или
API с сессией — поэтому без GUI гейт закрыть нечем.

`exec_plugin` — единственный инструмент, который исполняет код и **выключен по
умолчанию**: без `--allow-agent-exec` приходит `E_AGENT_EXEC_DENIED`. Он идёт
мимо пайплайна и мимо гейта, поэтому компенсирующих мер тут три: явный opt-in,
изолятор для плагинов агента (любая компонента пути равна `agent-plugins`) и
append-only журнал `<runs-dir>/agent-exec.jsonl`. В журнал пишутся две строки
на запуск: намерение **до** запуска и результат после. Не записалось намерение —
плагин не запускается вовсе (`E_AGENT_EXEC_AUDIT`). Одновременных запусков не
больше 4, дальше `E_AGENT_EXEC_BUSY`, а не очередь. Требование `E_GATE_REQUIRED`
на `exec_plugin` **не распространяется**: у инструмента нет шага, где человек
ответил бы «нет», и сделать `waiting_human` на нём технически нельзя. Это
осознанная дыра, а не забытая проверка; закрыть её можно, только дав агенту
средство попросить человека — и это отдельная работа.

MCP ограничивает plugin refs и `file_ref` рабочей директорией, отклоняет `any_host`,
loopback/private hosts и symlink escape. Это policy boundary, а не замена изоляции ОС.
Подробности: [docs/mcp.md](docs/mcp.md).

## Плагины и реестр

Встроенные модули `core/*` — доверенный код в процессе ядра: `human_gate`
(шлюз) и `text_stats` (метрики текста). Их список закрыт, в `permissions` они
ничего не заявляют, а наружу не ходят. Остальное — плагины: отдельный процесс,
JSON по stdin/stdout, контракт и разрешения в манифесте:

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
wedra demo [--runs-dir=<dir>]                    # автономная цепочка: ноль git, ноль Python, ноль сети
wedra pipeline validate <file.yaml> [--json]
wedra pipeline lint <file.yaml>
wedra pipeline plan <file.yaml>
wedra pipeline run <file.yaml> [--yes] [--resume=<run_id>] [--deny-untrusted-plugins] [--allow-untrusted-plugins]
wedra runs list | runs show <run_id> | runs resume <run_id> <file.yaml> [--yes]
wedra approve <run_id> <step_id>                      # подтвердить намерение человека (TTY + код), решение шлёт GUI/API
wedra plugin create|validate|test|install|inspect|list|search
wedra pipeline install <name|file.yaml|url>
wedra registry validate --registry=registry.yaml
wedra gui [--port 8765] [--open] [--plugins=<dir>] [--pipelines=<dir>]
wedra mcp --plugins <dir> --workdir <dir> [--no-gui]
wedra check [--list|--fast|--census|--only=<step>]        # единая проверка проекта
```

### Проверка проекта: одна команда

`wedra check` прогоняет шаги в том же порядке, что и CI, и печатает одну
сводку с кодом возврата:

```text
wedra check            # весь набор: fmt, vet, mod, build, test, conformance,
                       # pipelines, plugins, registry
wedra check --fast     # быстрые шаги перед коммитом: fmt, vet, mod, registry
wedra check --race     # полный набор с -race, ровно как в CI
wedra check --list     # какие шаги есть
wedra check --only=test
wedra check --census   # тесты поштучно по пакетам: свой предел на пакет
wedra check --census --pkg=internal/journal
```

Отдельный `Makefile` для этого не нужен и на Windows не работал: его шаги
записаны на юниксовой оболочке (`test -z`, `for … do`), и каждый шаг
приходилось вбивать руками.

`--census` нужен для охоты за зависанием: пакеты идут по очереди, у каждого
свой предел, поэтому первый зависший видно сразу. Логи — в
`var/census/<пакет>.txt`, а при превышении предела дополнительно снимаются
таблица процессов и скриншот в `var/census/HANG-*/`. Машина сводки:
`var/check/last-check.json`.

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
