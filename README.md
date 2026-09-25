![WEDRA](banner.png)

[English](README.en.md) | Русский

# WEDRA v0.31a

Локальный оркестратор цепочек с человеком в петле. WEDRA принимает YAML-пайплайны, запускает плагины отдельными процессами, проверяет их контракт, пишет журнал и позволяет безопасно продолжить прерванный run.

Это инструмент для доверенного локального пользователя и заранее проверенных плагинов. Для недоверенных community-плагинов и публичного HTTP-сервиса WEDRA не является OS-песочницей.

- Product version: [`VERSION`](VERSION) — `0.31a`
- Protocol version: [`protocol/VERSION`](protocol/VERSION) — `0.2`
- Primary CLI: `wedra`
- Legacy compatibility CLI: `tool`

Product version и protocol version независимы. Текущий релиз использует одобренный letter-suffix `v0.31a`; правила — в [docs/versioning.md](docs/versioning.md).

## Возможности

- YAML pipeline с `input`, `bind`, `when`, `foreach`, `parallel_group`, `retry` и `human_gate`.
- Строгая проверка pipeline и plugin manifest с кодами issues, подсказками и machine-readable JSON.
- Plugin contract: JSON через stdin/stdout, типы и форматы, domain error против platform error.
- Локальный GUI и редактор, MCP stdio adapter, CLI и HTTP API.
- Журнал `journal.jsonl`, context snapshots, resume с проверкой identity пайплайна.
- Registry install с полными SHA-пинами для удалённых источников.
- Ограниченные HTTP body, CSV, retry, aggregate и parallel resources.
- Windows process-tree termination через Job Object; Unix process groups.
- CSP/security headers, allowlist environment и fail-closed MCP path/network checks.

## Быстрый старт из исходников

Требуется Go 1.22+ и Python 3.9+ в `PATH` для Python-плагинов.

```bash
go build -o wedra ./cmd/wedra
./wedra version
go test ./...
```

Проверить плагин и запустить локальный pipeline:

```bash
./wedra plugin validate plugins/community/csv_loader
./wedra plugin test plugins/community/csv_loader
./wedra pipeline validate examples/email_check.yaml
./wedra pipeline lint examples/email_check.yaml
./wedra pipeline plan examples/email_check.yaml
./wedra pipeline run examples/email_check.yaml
```

На `human_gate` введите `a`, чтобы принять решение, или `r`, чтобы отклонить его. `--yes` не обходит `approval: human` и `pipeline.gates: human_only`.

## CLI

```text
wedra pipeline validate <file.yaml> [--json]
wedra pipeline lint <file.yaml>
wedra pipeline plan <file.yaml> [--json]
wedra pipeline run <file.yaml> [--yes] [--resume=<run_id>]
wedra runs list
wedra runs show <run_id>
wedra runs resume <run_id> <file.yaml> [--yes]
wedra plugin validate <dir>
wedra plugin test <dir> [--conformance] [--json]
wedra plugin install <name>[@version]
wedra pipeline install <name|file.yaml|url>
wedra registry validate --registry=registry.yaml
wedra gui [--listen 127.0.0.1:8765]
wedra mcp --plugins <dir> --workdir <dir> [--no-gui]
```

`tool` сохраняется как compatibility surface. Новые пользовательские сценарии добавляются в `wedra`.

## Минимальный pipeline

```yaml
format_version: "0.2"
pipeline:
  name: example
  input:
    text: "hello"
  steps:
    - id: analyze
      plugin: text_analyzer
      bind:
        text: input.text
    - id: review
      plugin: core/human_gate
      form:
        - field: steps.analyze.result
          editable: true
          type: string
      actions: [accept, reject]
      on_reject: stop
```

`core/human_gate` — единственный встроенный plugin. Другие `core/*` отклоняются. Полный формат и коды ошибок находятся в [protocol/v0.2/PROTOCOL.md](protocol/v0.2/PROTOCOL.md) и [protocol/v0.2/ERRORS.md](protocol/v0.2/ERRORS.md).

## Plugins

Plugin — отдельный процесс. Он получает только объявленные входы, возвращает JSON-конверт и должен объявлять manifest:

```yaml
id: my_plugin
version: 0.1.0
platform_api: "^0.1"
runtime:
  type: python
  entry: main.py
  requires: []
input:
  text:
    type: string
output:
  result:
    type: string
permissions:
  network: []
  filesystem: none
  secrets: []
```

`permissions` — декларация и аудит, не OS sandbox. Секреты передаются только именами env-переменных, объявленными в manifest и pipeline. Если plugin объявляет `runtime.requires`, используются exact pins `package==version` и рядом обязателен `requirements.lock`.

Авторская документация: [docs/plugin-dev.md](docs/plugin-dev.md).

## Registry и install

`registry.yaml` содержит plugins и presets. Для удалённого source запись должна иметь полный 40/64-hex commit pin; установка проверяет фактический checkout и падает при несовпадении.

```bash
./wedra registry validate --registry=registry.yaml
./wedra plugin install text_analyzer
./wedra pipeline install email_check
```

Локальный source не является доверенным автоматически: при `--local-source` проверяются путь и pin, а при несовпадении install fail-closed.

## Secrets и сеть

Никогда не записывайте реальные ключи в YAML, fixtures, журналы или examples. В pipeline объявляются только имена:

```yaml
pipeline:
  secrets: [OPENAI_API_KEY]
```

Subprocess получает allowlist environment и только явно объявленные secrets. `network: deny` запрещает шаги с сетевыми permissions; `allow` передаёт плагину `WEDRA_NETWORK=allow` и оставляет декларацию в журнале.

## GUI и MCP

GUI запускается локально и использует HttpOnly session cookie для mutating API. В MCP агенту не выдаётся инструмент approval: при `waiting_human` человек открывает GUI и принимает решение в браузере.

MCP ограничивает plugin refs и `file_ref` рабочей директорией, отклоняет `any_host`, loopback/private hosts и symlink escape. Это policy boundary, а не замена изоляции ОС.

```bash
./wedra gui --listen 127.0.0.1:8765
./wedra mcp --plugins "$PWD/plugins" --workdir "$PWD" --no-gui
```

## Security model

- WEDRA запускает subprocess с правами текущего пользователя.
- `permissions`, CSP и проверки путей не являются sandbox.
- Не запускайте недоверенные community plugins или принимайте untrusted MCP без отдельной OS-изоляции.
- Не передавайте секреты через URL, pipeline input или registry metadata.
- Сообщения о безопасности: [SECURITY.md](SECURITY.md).

## Release assets

Стабильные сборки публикуются в [GitHub Releases](https://github.com/wykserdex/wedra/releases). Release workflow собирает:

- `wedra-{linux,darwin,windows}-{amd64,arm64}`;
- legacy `tool-{linux,darwin,windows}-{amd64,arm64}`;
- `wedragui-windows-{amd64,arm64}.exe`;
- conformance package и `SHA256SUMS`.

Тег релиза `v0.31a` (одобренный letter-suffix) должен совпадать с `VERSION`. Перед публикацией выполняются tests, vet, `govulncheck`, registry validation, conformance и cross-build checks.

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

`internal/core` и `cmd/tool` — transitional compatibility layers. Не переименовывайте канонические директории без отдельного RFC/ADR и migration plan; структура описана в [docs/architecture.md](docs/architecture.md).

## Документация и история

- [Quickstart](docs/quickstart.md)
- [Architecture](docs/architecture.md)
- [Versioning](docs/versioning.md)
- [Plugin development](docs/plugin-dev.md)
- [MCP](docs/mcp.md)
- [Governance](GOVERNANCE.md)
- [Contributing](CONTRIBUTING.md)
- [Полная история изменений](CHANGELOG.md)

Лицензия и условия использования публикуются вместе с репозиторием.
