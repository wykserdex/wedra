# orchestrator — скелет MVP v10

Локальный оркестратор цепочек с человеком в петле. M1–M5 закрыты, M6 GUI отложен. **v10** — архитектурный рефактор по фидбеку: god-package `internal/core` разбит на домены, `runs/` → `var/runs/` + `RunStore`, добавлены JSON Schema, conformance, official/community.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов (6+4 community), 8 пайплайнов, 0 провалов воронки, ядро 8.5–9/10. Их слова:

> «каждый кусок можно независимо написать, протестировать и заменить» ·
> «ошибки говорят буквально, какой порт и почему несовместим» ·
> «контракт честный, тот же результат что в ране и в echo | python3 main.py»

```
orchestrator/
├── PROTOCOL.md          # контракт ядро↔плагин (v0.2)
├── protocol/            # версионированный протокол + JSON Schema
│   ├── versions/v0.1.md, v0.2.md
│   ├── schemas/v0.2/    # manifest, request, response
│   └── README.md        # lifecycle, envelope план v0.3
├── schemas/pipeline/    # pipeline v0.2 JSON Schema
├── cmd/
│   ├── tool/            # старый бинарь (совместимость)
│   └── orchestrator/    # новый бинарь: orchestrator pipeline|plugin
├── internal/
│   ├── pipeline/        # модель, парсер, валидатор, планер
│   ├── execution/       # runner, scheduler, retry
│   ├── plugin/          # registry, process, transport, fileref
│   ├── journal/         # writer, reader, store (RunStore)
│   ├── gate/            # service, terminal (human_gate)
│   ├── context/         # store, binding
│   ├── common/          # util (KindOf, Basename, Truncate)
│   ├── cli/             # root, run, validate, plugin commands
│   └── core/            # shim-фасад для совместимости (93 теста)
├── plugins/
│   ├── official/        # 5 официальных (syntax, disposable, llm x3)
│   ├── community/       # 4 community (text_analyzer, dir_lister, csv_loader, email_triage)
│   └── *.py             # плоская копия для совместимости (deprecated)
├── pipelines/           # 8 пайплайнов (все зелёные)
├── examples/
│   ├── pipelines/       # примеры для доки
│   └── plugins/         # text_analyzer как пример
├── conformance/         # conformance fixtures + README
├── var/runs/            # журналы (было runs/, теперь gitignore var/runs/)
└── .github/workflows/ci.yml # CI (go test, vet, build)
```

## Быстрый старт

> Хочешь писать СВОЙ плагин? → `TUTORIAL_PLUGINS.md` (15 мин, выверен на 4 внешних авторах).

Go ≥1.21, Python ≥3.9.

```bash
go build -o orchestrator ./cmd/orchestrator   # новый CLI
go build -o tool ./cmd/tool                   # старый (совместимость)
go test ./...                                 # 93 теста
go vet ./...

./orchestrator plugin validate plugins/csv_loader
./orchestrator plugin test plugins/csv_loader
./orchestrator pipeline validate pipelines/email_check.yaml
./orchestrator pipeline plan pipelines/email_triage_chain.yaml
./orchestrator pipeline run pipelines/email_check.yaml --yes
# var/runs/<id>/journal.jsonl + context.json

# совместимость:
./tool validate pipelines/email_check.yaml
./tool run pipelines/email_check.yaml --yes
```

LLM-пайплайны:
```bash
LLM_MOCK=1 ./orchestrator pipeline run pipelines/llm_text_chain.yaml --yes
GEMINI_API_KEY=... LLM_OAI_API_KEY=... ./orchestrator pipeline run pipelines/llm_same_provider.yaml --yes
```

## Что изменилось в v10 (архитектурный фидбек)

**Приоритет из фидбека:**
1. **Разбить god-package `internal/core`** → `pipeline/`, `execution/`, `plugin/`, `journal/`, `gate/`, `context/`, `common/`, `cli/` + shim в `core/` для совместимости. 93 теста зелёные.
2. **runs/ → var/runs/ + RunStore** — `internal/journal/store.go` интерфейс `RunStore` (Create, AppendEvent, SaveArtifact, Load), `FilesystemStore` для MVP, план SQLite/S3 для сервера. `.gitignore` теперь `var/runs/`.
3. **JSON Schema** — `protocol/schemas/v0.2/manifest.schema.json`, `request.schema.json`, `response.schema.json`, `schemas/pipeline/v0.2.schema.json`. Машинно-читаемый контракт для SDK/IDE.
4. **Conformance suite** — `conformance/README.md` + фикстуры `internal/core/testdata/plugins/` (echo_ok, failer, crasher, bad_proto, contract_breaker, leaker, type_drifter, file_ref_echo, sleeper, retry_flaky). Проверки: handshake, unknown message type, bad JSON, timeout, crash→platform, мусор→protocol_violation, большой output, несовместимая версия.
5. **official/community split** — `plugins/official/` (5) и `plugins/community/` (4) + плоская копия для совместимости. `examples/pipelines/` и `examples/plugins/`.
6. **CLI: cmd/orchestrator/main.go** — `orchestrator pipeline run|validate|plan` и `plugin validate|test|create|inspect`, backward compat `run`/`validate`. `internal/cli/` теперь точка сборки.
7. **Протокол версионирован** — `protocol/versions/v0.1.md`, `v0.2.md`, `protocol/README.md` с lifecycle и планом envelope v0.3 (protocol_version, request_id, cancel, handshake, streaming).

## SDK: plugin.test.yaml

```yaml
tests:
  - name: mailinator → disposable=true
    input: { email: "user@mailinator.com" }
    expect:
      output:
        disposable: true
        domain: { contains: "mailinator" }
  - name: без ключа → понятная ошибка
    env: { LLM_OAI_API_KEY: "" }
    input: { prompt: "x" }
    expect: { status: error, error: { code: no_api_key } }
```

Матчеры: литерал (глубокое равенство), `{ present: true }`, `{ contains: "..." }` (строка и массив), `{ type: "..." }`, `{ equals: ... }`. Enforce: незадекларированное поле → warning, пропавший обязательный или дрейф типа → провал.

## Что уже работает (M1-M5)

- subprocess stdin/stdout JSON + exit codes 0/1/≥2
- Shared Context `steps.<id>`, EnforceOutput
- Validate до запуска: пути, типы, форматы, skip-безопасность, bind на несуществующий порт, literal format
- Policies stop|skip|retry, foreach per-item
- human_gate: form, правки с проверкой типа, materialize с префиксом при коллизии basename, truncate 500, crash→platform hint
- 3 LLM-адаптера (Gemini, OpenAI-совместимый, Anthropic) + mock
- plugin create → test → validate триада
- file_ref warning до запуска с подсказкой «есть от корня»
- Journal var/runs/<id>/journal.jsonl + context.json

## Сознательные упрощения

| Долг | Куда |
|---|---|
| Нет --resume | M2→M4, §4.8 |
| Секреты только env | §4.9 |
| permissions L0 декларативны | §4.5 |
| foreach только input.* | M6 |
| human_gate выходы не типизированы | #19 backlog |
| Execution пока делегирует в core.Run, полный перенос в M6 | v10 → v11 |

## Тесты

`go test ./...` — 93 теста: контекст, валидация, экзекутор, enforce, раннер (foreach, retry, platform stop, gate), SDK (plugin.test.yaml на фикстурах и на всех 10 шипленных плагинах).

## Следующие шаги

- M5 закрыт (8.5–9/10), M6 GUI отложен — дистрибуция = dev-чаты + репа
- v10: архитектурный рефактор (этот релиз)
- v11: полный перенос логики из core shim в новые пакеты, SQLite RunStore, conformance runner в CI, plugin registry API
