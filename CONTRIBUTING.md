# Contributing to WEDRA

WEDRA is a young project with a public, compatibility-conscious contribution
process. Read [GOVERNANCE.md](GOVERNANCE.md) before changing protocol,
registry policy, CLI compatibility, or repository structure.

## Development workflow

1. Use Go 1.22 or newer and Python 3 for plugin fixtures.
2. Build the primary CLI with `go build -o wedra ./cmd/wedra`.
3. Run `go test ./... -count=1` and `go vet ./...`.
4. Keep generated run data under ignored `var/runs/`; never commit journals,
   `.wedra` locks, binaries, or local registry state.
5. Use `wedra` for new documentation and CI checks. `tool` is a compatibility
   command and should not define new behavior.

## Product and protocol versions

The product version is the release value in the root `VERSION` file. The
protocol version is independent and lives in `protocol/VERSION`; the complete
rules are in [docs/versioning.md](docs/versioning.md). Do not reuse a released
tag or infer a new product version from an old `v9`/`v10` alias.

A protocol, registry, or layout change requires a public proposal, a migration
note, and compatibility tests. The canonical repository layout is documented
in [docs/architecture.md](docs/architecture.md) and is frozen against parallel
renames until an accepted ADR.

## Registry admission

The registry is `registry.yaml` in the repository root, with schema version
`0.1`. A plugin or preset enters the supported registry only after green CI,
including `wedra registry validate`.

### Plugin checklist

1. **Манифест** `plugin.yaml`:
   - `id` — обязан совпасть с именем записи в реестре (CI проверяет);
   - `runtime`, `input`/`output` порты;
   - **`permissions` — честно**:
     - `network: [{host, port}]` — КАЖДЫЙ сетевой вызов, который плагин делает
       (пусто = плагин сетевой быть не будет, `network: deny` пайплайна его запретит);
     - `secrets: [ENV_KEY]` — КАЖДЫЙ env-ключ, который плагин читает.
2. **Конформность** `plugin.test.yaml` — минимум 3 кейса:
   - happy path (`status: ok`, поля вывода);
   - доменная ошибка (`error.code`, `retryable` — ожидаемые значения);
   - битый JSON на входе (exit 2 — платформенная ошибка).
   Реальных ключей в тестах быть не должно — mock-режим (паттерн LLM-плагинов:
   `LLM_MOCK=1`), сеть — только если тестируемая (MX-запись и т.п.).
3. Локальные проверки:
   ```
   wedra plugin validate <dir>
   wedra plugin test <dir>
   ```
4. **PR**: каталог `plugins/{official,community}/<name>/` + запись в
   `registry.yaml` (`source`, `path`, `version` = тег текущего релиза,
   `description` — что делает, одной строкой).
5. CI: `registry validate` прогоняет манифест, id, **все** конформные тесты.
   Красный = в реестре не оказаться.

### Preset checklist

1. `examples/<name>.yaml`:
   - `format_version`, `steps`;
   - `secrets: [KEY]` — все ключи, которые цепочке нужны;
   - `network: deny` — если цепочка сетью пользоваться не должна.
2. `wedra pipeline validate examples/<name>.yaml` — зелёный.
3. **PR**: файл + запись пресета в `registry.yaml`.

### Reviewer checklist

- `permissions` честные: код делает ровно то, что заявлено в манифесте (и не больше).
- В коде/тестах/примерах — ни одного значения секрета (только имена).
- Ошибки: доменные vs платформенные, `retryable` по смыслу.
- `description` — что делает и что нужно, без маркетинга.

## Version and security links

- Product version: root `VERSION` and [docs/versioning.md](docs/versioning.md).
- Protocol version: `protocol/VERSION` and `protocol/CHANGELOG.md`.
- Registry source pins: `registry.yaml` (`version` + `commit`), never `main`.
- Security reports: [SECURITY.md](SECURITY.md).
- Governance and breaking changes: [GOVERNANCE.md](GOVERNANCE.md).
