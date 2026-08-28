# orchestrator v0.12 — CLI focus (мясо, не косметика)

Локальный оркестратор цепочек с человеком в петле. M1–M5 закрыты, M6 GUI **отложен** — ставка на CLI. Честная версия: **v0.12**.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов, 8+1 пайплайнов, 0 провалов, ядро 8.5–9/10.

> «каждый кусок можно независимо написать, протестировать и заменить» · «контракт честный»

```
orchestrator/
├── VERSION              # 0.12
├── PROTOCOL.md          # контракт v0.2 + v0.12 foreach steps.*
├── internal/
│   ├── pipeline/        # модель, парсер, валидатор (input.* + steps.* foreach), планер
│   ├── execution/       # runner (resume, two-phase foreach), scheduler
│   ├── plugin/          # registry, process, transport, fileref
│   ├── journal/         # writer (New + OpenAppend), reader, store (RunStore)
│   ├── gate/            # service, terminal + fix #19 typing
│   ├── context/         # store, binding (nested input.row.name)
│   ├── cli/             # pipeline|plugin|runs|gui|version
│   ├── api/             # REST API (M6, отложен)
│   └── core/            # shim-фасад (93 теста)
├── web/static/          # GUI scaffold (отложен)
├── plugins/official/    # 5, community/ 4
├── pipelines/           # 9 (8 старых + csv_foreach — демо foreach steps.*)
└── var/runs/            # журналы + --resume
```

## Быстрый старт (CLI — мясо)

```bash
go build -o orchestrator ./cmd/orchestrator
./orchestrator version   # v0.12
go test ./...            # 93 теста

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
- [ ] v0.13: полный перенос core → execution/journal/gate, SQLite RunStore, `pipeline lint` с file_ref проверкой до запуска (сейчас warning)
- [ ] v0.14: `foreach` как шаг (а не только pipeline), параллельные ветки, `when:` условия
- [ ] v1.0: GUI full (import YAML, SVG, run из GUI, human_gate форма) + маркетплейс v1

## Тесты

`go test ./...` — 93 теста PASS, `csv_foreach` зелёный (ok=2), resume — все элементы уже пройдены.

## Версионирование

- v0.9 (ex v9), v0.9.1 (ex v9.1), v0.10 (ex v10), v0.11 (GUI scaffold), v0.12 (CLI focus)
- Дальше: v0.13, v0.14... v1.0 — когда CLI meat закрыт + GUI + маркетплейс
