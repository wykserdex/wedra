# orchestrator v0.16 — install-путь: «взял и использовал»

Локальный оркестратор цепочек с человеком в петле. M1–M5 закрыты, M6 GUI **отложен** — ставка на CLI. Честная версия: **v0.16**.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов, 8+1 пайплайнов, 0 провалов, ядро 8.5–9/10.

> «каждый кусок можно независимо написать, протестировать и заменить» · «контракт честный»

```
orchestrator/
├── VERSION              # 0.16
├── PROTOCOL.md          # контракт v0.2 + v0.12 foreach steps.*
├── registry.yaml        # v0.16: реестр v0.1 (plugins + presets), формат заморожен
├── internal/
│   ├── pipeline/        # модель (secrets), парсер, валидатор, планер
│   ├── execution/       # runner (resume, two-phase foreach, secrets preflight), scheduler
│   ├── plugin/          # registry, process, transport, fileref
│   ├── registry/        # v0.16: реестр, install, pin-контракт (RefToDir)
│   ├── journal/         # writer (New + OpenAppend), reader, store (RunStore)
│   ├── gate/            # service, terminal + fix #19 typing
│   ├── context/         # store, binding (nested input.row.name)
│   ├── cli/             # pipeline|plugin|runs|gui|version + install
│   ├── api/             # REST API (M6, отложен)
│   └── core/            # shim-фасад (107 тестов)
├── web/static/          # GUI scaffold (отложен)
├── plugins/official/    # 5, community/ 5
├── pipelines/           # 10
└── var/runs/            # журналы + --resume
```

## Быстрый старт (CLI — мясо)

```bash
go build -o orchestrator ./cmd/orchestrator
./orchestrator version   # v0.16
go test ./...            # 107 тестов

# плагины
./orchestrator plugin validate plugins/csv_loader
./orchestrator plugin test plugins/csv_loader   # 7 PASS
./orchestrator plugin test plugins/email_triage # 10 PASS

# пайплайны
./orchestrator pipeline validate pipelines/email_check.yaml
./orchestrator pipeline lint pipelines/csv_foreach.yaml
./orchestrator pipeline plan pipelines/csv_foreach.yaml

# v0.12: foreach по результату шага (было только input.*)
./orchestrator pipeline run pipelines/csv_foreach.yaml --yes
# фаза 1: load rows → фаза 2: foreach row → check → review, ok=2

# v0.12: --resume
./orchestrator runs list
./orchestrator runs show <run_id>
./orchestrator pipeline run pipelines/email_triage_chain.yaml --yes --resume=<run_id>
./orchestrator runs resume <run_id> pipelines/email_triage_chain.yaml --yes

# совместимость tool
./tool run pipelines/csv_foreach.yaml --yes
./tool runs list
```

## v0.16: install-путь («взял и использовал»)

Плагин или пресет — не из локального каталога, а из реестра (`registry.yaml`,
включён в это репо):

```bash
# пресет из реестра + АВТОУСТАНОВКА его плагинов в plugins/
./orchestrator pipeline install email_check
./orchestrator pipeline run pipelines/email_check.yaml --yes

# плагин из реестра (или с пином версии)
./orchestrator plugin install text_analyzer
./orchestrator plugin install my_summarizer@v0.16

# свой реестр (оффлайн: каталог с registry.yaml) — без сети
./orchestrator pipeline install my_preset --registry=./my_registry
```

Ссылки на плагины в пайплайне (v0.16, назад-совместимо):

| форма | пример | разрешение |
|---|---|---|
| локальный путь | `plugin: plugins/community/text_analyzer` | как раньше |
| реестровое имя | `plugin: text_analyzer` | из `plugins/text_analyzer` (уже установлен) |
| pin | `plugin: text_analyzer@v0.16` | то же + версия из `.wedra` |

`pipeline install` сам докачает недостающие плагины, переключит на нужную
версию под пин и провалидирует совместимость. Не установлен — понятная ошибка
с командой установки.

**Secrets** — пайплайн объявляет имена env-переменных, без значений:

```yaml
format_version: "0.2"
pipeline:
  name: llm_report
  secrets: [OPENAI_API_KEY]   # имена, не значения
  steps:
    - id: analyze
      plugin: llm_openai
```

`validate` предупредит, `run` упадёт до любого эффекта, если ключ не
экспортирован. Значения в YAML не живут.

## Что нового в v0.16 (install-путь)

- **Реестр** `registry.yaml` (v0.1, формат заморожен) — в корне репо: `plugins` +
  `presets`, `source`/`path`/`version`/`description`. Один репо = реестр +
  источник; позже реестр выносится в отдельный репо — формат не меняется.
- **`plugin install <name>[@version]`** — clone из `source`, валидация,
  lock-файл `.wedra` (name/source/version) в каталоге плагина.
- **`pipeline install <name|file|url>`** — пресет → `pipelines/<name>.yaml` +
  автоустановка плагинов + валидация.
- **`name` / `name@version`** в `plugin:` (наряду с локальными путями).
- **`secrets:`** в пайплайне — имена env, preflight в раннере.
- **Оффлайн**: `--registry=<каталог>` с локальным source — без сети.
- Тесты: **107 PASS**.

## Что нового в v0.15 (честный релиз)

**1. `SQLiteStore` → `JsonStore`** — честное имя: это pure Go JSON-файл (`var/runs/runs.db`), не SQLite. `loadDB` больше не глотает ошибки: битый индекс — явная ошибка при записи, чтения деградируют в FS-журнал (источник истины). Убраны мёртвые поля `dbRun`. CLI: `--store=sqlite` → `--store=json`.

**2. `common.Truncate` rune-safe** — `s[:n]` больше не режет UTF-8 символ пополам (мусор в русских сообщениях об ошибках). Одна функция в common вместо трёх локальных копий.

**3. Двойной кэш в Engine убран** (`core.Engine`, `plugin.Engine`) — один `Cache`.

**4. `GateUI`-интерфейс** — канал ввода human_gate теперь заменяемый (`NewServiceWithUI`). Шов для GUI/API (M6).

**5. Одна история версий** — VERSION, README, CHANGELOG, теги.

## Что нового в v0.12 (CLI focus)

**1. foreach steps.* — закрыт #12**
- Было: только `input.*` — нельзя «прочитал CSV → итерирую по строкам»
- Стало: `foreach: steps.load.rows` — двухфазный ран
  - preSteps (0..srcID) выполняются один раз, получают массив
  - foreachSteps (srcID+1..) выполняются per-item
- Валидатор: `foreach` теперь `input.*` ИЛИ `steps.<id>.<field>`, проверяет существование шага-источника
- Демо: `pipelines/csv_foreach.yaml` — `load` (csv_loader) → `foreach: steps.load.rows` → `check` (text_analyzer на `input.row.name`) → `review` (gate)

**2. --resume — журнал как фундамент**
- `RunOptions.Resume` + `journal.OpenJournalAppend`
- Загружает `var/runs/<id>/context.json`, парсит `journal.jsonl` → `max item_index`, пропускает пройденные
- CLI: `orchestrator pipeline run --resume=<id>`, `orchestrator runs resume <id> <yaml>`

**3. runs CLI**
- `orchestrator runs list [var/runs]` — список прогонов с pipeline name и events count
- `orchestrator runs show <id>` — полный журнал + context snapshot
- `tool runs list/show` — совместимость

**4. human_gate typing fix #19**
- Если в `form` нет `type`, тип выводится из источника `ctx.Get(field)` (kindOf)
- Правка валидируется по выведенному типу, сообщение: «тип X не подходит под Y (выведен из ...)»

**5. context binding — nested**
- Поддержка `input.row.name` где `row` — объект (foreach item). `Ctx.Get` уже умел, валидатор теперь возвращает any для вложенных путей

**6. Версионирование честное 0.x**
- v10 → v0.10, v0.11 → GUI scaffold (отложен), v0.12 → CLI meat
- GUI — косметика, отложен до v1.0, фокус — мясо: foreach steps.*, resume, runs, typing

## M6 DoD (обновлён, GUI в последнюю очередь)

- [x] v0.11 scaffold GUI (отложен)
- [x] v0.12 CLI meat: foreach steps.*, resume, runs list/show, gate typing
- [x] v0.13: полный перенос core → execution/journal/gate, JsonStore, `pipeline lint` с file_ref проверкой до запуска
- [x] v0.15: честный релиз — JsonStore (честное имя) + ошибки loadDB, rune-safe Truncate, GateUI, один кэш Engine
- [x] v0.16: **install-путь** — реестр v0.1, `plugin install`, `pipeline install` (автоустановка плагинов), pin `name@version`, `secrets:`, оффлайн
- [ ] v0.17: `foreach` как шаг (а не только pipeline), параллельные ветки, `when:` условия
- [ ] v1.0: GUI full (import YAML, SVG, run из GUI, human_gate форма) + маркетплейс v1

## Тесты

`go test ./...` — 107 тестов PASS, `csv_foreach` зелёный (ok=2), resume — все элементы уже пройдены, install-сценарии покрыты e2e.

## Версионирование

- v0.9 (ex v9), v0.9.1 (ex v9.1), v0.10 (ex v10), v0.11 (GUI scaffold), v0.12 (CLI focus), v0.13 (честный перенос), v0.14 (JsonStore — тогда ещё назывался SQLiteStore, в v0.15 переименован честно), v0.15 (честный релиз), v0.16 (install-путь)
- Дальше: v0.17 (foreach как шаг, параллельные ветки, when:) → v1.0 — GUI + маркетплейс
