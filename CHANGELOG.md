# Changelog — честная 0.x

## v0.12.1 (2026-08-28) — patch после аудита v0.12

Аудит v0.12 нашёл 6 проблем — все закрыты:

- **foreach steps.* full #12** — добавлен `after_foreach: true`. Теперь 3 фазы: preSteps (до src), loopSteps per-item, postSteps один раз после всех. Агрегаты `steps.<id>_all` доступны в post-фазе. Новый пайплайн `pipelines/csv_foreach_summary.yaml` — load → check (x2) → summary (once, gate с `steps.check_all`).
- **validator** — поддерживает `steps.<id>_all` (array) и `steps.<id>.<field>_all`, не ругается на агрегаты в form.
- **runner resume** — live-тест: `steps.*` массив читается из `context.json` снапшота, preSteps пропускаются, postSteps выполняются даже если все items уже пройдены. Агрегация при partial resume мержится из снапшота (`steps.<id>_all[:startIdx] + новые`).
- **internal/api/server.go** — 4 бага закрыты:
  - PUT `/api/pipelines/{file}` раньше писал старый контент обратно (`os.WriteFile raw`), теперь `io.ReadAll r.Body + LoadPipelineFileFromBytes` валидация.
  - health `/api/health` хардкод `0.11` → читает `VERSION` файл + const `Version`.
  - plan `/api/plan/pipeline` был алиасом validate → теперь строит DAG `{nodes:[id,plugin,phase,bind,after_foreach], edges:[from,to,via]}` с фазами pre/foreach/post.
  - Routes fallback если `web/static` нет → не падает, отдаёт текст "GUI postponed".
  - GUI version string теперь из `api.Version`.
- **CI** — `go.yml` удалён, `ci.yml` с workflow scope PAT: go vet, fmt, test, build, plugin validate/test, pipeline validate, foreach+after_foreach, resume, schemas.
- **93 теста PASS**, `csv_foreach_summary` зелёный, resume live проверен.

## v0.12 (2026-08-28) — CLI focus, мясо, не косметика (M6 meat)
- **GUI отложен** — косметика, не мясо. Ставка на CLI.
- **foreach steps.*** — закрыт #12: теперь `foreach: steps.load.rows` работает (двухфазный ран: preSteps → foreach). Демо `pipelines/csv_foreach.yaml` — CSV → per-row text_analyzer → gate, ok=2.
- **--resume** — `orchestrator pipeline run <yaml> --resume=<run_id>` и `orchestrator runs resume <run_id> <yaml>`. Загружает `var/runs/<id>/context.json` и пропускает пройденные `item_index` из `journal.jsonl`.
- **runs CLI** — `orchestrator runs list`, `runs show <id>`, `runs resume` + `tool runs list/show` (совместимость)
- **human_gate typing fix #19** — если в `form` нет `type`, тип выводится из источника `ctx.Get(field)` (kindOf), правка валидируется по выведенному типу
- **pipeline validator** — `foreach` теперь `input.*` ИЛИ `steps.<id>.<field>`, проверка существования шага-источника
- **execution/runner** — начал перенос логики из `core/runner.go` в `execution/`, пока делегирует в core (полный перенос в v0.13)
- **context binding** — поддержка `input.<item>.<field>` (nested) для foreach-объектов
- **VERSION 0.12**, 93 теста PASS, `csv_foreach` зелёный

## v0.11 (2026-08-28) — M6 GUI scaffold (отложен в v0.12)
- `internal/api` — REST API: /api/health, plugins, pipelines, runs, validate/plan
- `web/static` — GUI: drag-and-drop, live YAML, validate, JSON на линиях, экспорт YAML
- `internal/cli/gui.go` — `orchestrator gui [--port 8080] [--open]`
- `VERSION` файл = 0.11
- Версионирование: v10 → v0.10, v11 → v0.11 (честно, не бьём до 100+)
- M6 DoD частично: сборка без YAML работает, live-sync экспорт, JSON на линиях

## v0.10 (2026-08-28) — бывший v10, архитектурный рефактор
- Разбит `internal/core` на pipeline/execution/plugin/journal/gate/context/common/cli + shim
- `runs/` → `var/runs/` + RunStore интерфейс (FilesystemStore)
- JSON Schema: protocol/schemas/v0.2 + schemas/pipeline/v0.2
- Conformance suite + fixtures
- official/community split + examples
- CLI `orchestrator` binary + backward compat `tool`
- Протокол версионирован (v0.1, v0.2, envelope план v0.3)
- 10 плагинов, 8 пайплайнов, 93 теста PASS

## v0.9.1 (бывший v9.1) — быстрые фиксы
- csv_loader (7 тестов) + email_triage (10 тестов) закоммичены
- README TODO, gate truncate 120→500, crash→platform hint
- CI workflow локально (.github/workflows/ci.yml)

## v0.9 (бывший v9) — критичные баги
- #9 gate collision (basename → <step_id>_<basename>)
- #10 optional bind = warning, не error
- #11 platform:<code> на exit>=2
- #13 format_version strict

## v0.8 (M5)
- 4 внешних автора, 0 провалов, 8.5–9/10, 10 плагинов, 8 пайплайнов
