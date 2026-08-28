# orchestrator v0.11 — M6 GUI (честная 0.x)

Локальный оркестратор цепочек с человеком в петле. M1–M5 закрыты, **M6 начат**. Версионирование честное: **v0.10 = бывший v10**, **v0.11 = M6 GUI scaffold**. Больше не бьём до 100+.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов, 8 пайплайнов, 0 провалов, ядро 8.5–9/10.

> «каждый кусок можно независимо написать, протестировать и заменить» · «контракт честный»

```
orchestrator/
├── VERSION              # 0.11
├── PROTOCOL.md          # контракт v0.2
├── protocol/            # версионированный протокол + JSON Schema
├── schemas/pipeline/    # pipeline v0.2 JSON Schema
├── cmd/
│   ├── tool/            # старый CLI (совместимость)
│   └── orchestrator/    # новый: pipeline|plugin|gui|version
├── internal/
│   ├── pipeline/        # модель, парсер, валидатор, планер
│   ├── execution/       # runner, scheduler
│   ├── plugin/          # registry, process, transport, fileref
│   ├── journal/         # writer, reader, store (RunStore)
│   ├── gate/            # service, terminal
│   ├── context/         # store, binding
│   ├── common/          # util
│   ├── cli/             # root, run, validate, plugin, gui
│   ├── api/             # REST API для GUI (M6)
│   └── core/            # shim-фасад (93 теста)
├── web/static/          # GUI: index.html + app.js (drag-and-drop, live YAML, JSON на линиях)
├── plugins/official/    # 5 официальных
├── plugins/community/   # 4 community
├── pipelines/           # 8 пайплайнов
├── examples/            # примеры
├── conformance/         # conformance suite
└── var/runs/            # журналы
```

## Быстрый старт

```bash
go build -o orchestrator ./cmd/orchestrator
go test ./...   # 93 теста
./orchestrator version
./orchestrator plugin validate plugins/csv_loader
./orchestrator pipeline validate pipelines/email_check.yaml
./orchestrator pipeline run pipelines/email_check.yaml --yes
./orchestrator gui --port 8080 --open   # M6: http://localhost:8080
```

GUI M6 (v0.11 scaffold):
- Левая панель: плагины (official/community), пайплайны, прогоны (var/runs)
- Центр: канвас drag-and-drop, ноды можно двигать, коннекты через bind
- Правая: свойства ноды (bind порт→путь, on_error, form), Live YAML, валидация, JSON на линиях (context.json)
- Экспорт YAML, Validate (DAG), Plan — через `/api/validate/pipeline`
- Run пока через CLI (план v0.12 — запуск из GUI)

API:
- `GET /api/health` → version, protocol
- `GET /api/plugins` → список
- `GET /api/pipelines` → список
- `GET /api/runs` → список прогонов
- `GET /api/runs/<id>` → events + context snapshot
- `POST /api/validate/pipeline` (yaml) → errors/warnings

## Что в v0.10 (бывший v10)

1. Разбит `internal/core` на 8 пакетов + shim, 93 теста зелёные
2. `runs/` → `var/runs/` + `RunStore` интерфейс
3. JSON Schema: manifest, request, response, pipeline
4. Conformance suite + fixtures
5. official/community split + examples
6. CLI `orchestrator` binary + backward compat `tool`
7. Протокол версионирован (v0.1, v0.2, envelope план v0.3)

## Что в v0.11 (M6 старт)

- `internal/api/server.go` — REST API
- `web/static/index.html, app.js` — GUI scaffold (drag-and-drop, live YAML, валидация, JSON на линиях)
- `internal/cli/gui.go` — `orchestrator gui [--port --open]`
- `VERSION` файл = 0.11
- Версионирование честное 0.x: v0.10, v0.11, v0.12... вместо 10,11,100+

## M6 DoD по ТЗ

- [ ] Цепочка собирается с нуля без касания YAML (drag-and-drop) — **в v0.11 scaffold, работает добавление нод, bind, экспорт**
- [ ] Live-sync round-trip без потерь — **в v0.11: GUI→YAML экспорт, YAML→GUI импорт TODO v0.12**
- [ ] JSON на линиях — **в v0.11: клик по прогону показывает context.json + events**
- [ ] human_gate с формами — **в v0.11: form редактируется как JSON, в v0.12 — нормальная форма**

Следующие шаги M6 (v0.12):
- Импорт YAML в канвас (парсер → ноды)
- SVG линии для bind (steps.* → steps.*)
- Запуск пайплайна из GUI (Run --yes) + стриминг journal.jsonl (SSE)
- human_gate в GUI: форма с editable полями, accept/reject
- Wails обёртка (десктоп бинарь) — опционально, пока web достаточно

## Тесты

`go test ./...` — 93 теста, `go vet` чист.

## Версионирование

- Было: v9, v9.1, v10 → честно 0.x: v0.9, v0.9.1, v0.10
- Сейчас: v0.11 (M6 scaffold)
- Дальше: v0.12 (M6 full GUI), v0.13, ... v1.0 — когда GUI + маркетплейс + 5 внешних плагинов
