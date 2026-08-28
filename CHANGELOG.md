# Changelog — честная 0.x

## v0.13 (2026-08-29) — честный перенос логики из монолита (ответ на фидбек v10)

Главный фидбек: v10 рефакторинг косметический, логика осталась в `internal/core` (~2.5k строк), а новые пакеты — заглушки. В v0.13 довёл до конца:

- **execution/runner.go — real move**: вся логика из `core/runner.go` перенесена в `execution`. Теперь `execution.Run` — источник правды (3 фазы pre/loop/post, agg `steps.<id>_all`, resume через `RunStore`). `core/runner.go` — тонкий shim `→ execution.Run`. Убран хрупкий `strings.Contains("\"type\":\"item_end\"")` — теперь `journal.FilesystemStore.MaxItemIndex()` через `Reader.Events()` с нормальным `json.Unmarshal` и проверкой `ev["type"]=="item_end"`.
- **journal/store.go — real impl**: `FilesystemStore` с методами `Create`, `OpenAppend`, `AppendEvent`, `SaveArtifact`, `LoadContext`, `MaxItemIndex` (без хрупкого парсинга). Добавлен `SQLiteStore` заготовка (делегирует в FS, DBPath для будущего). `Journal.Event` теперь пишет `type` + `ts` + поля, `Snapshot` — `context.json`.
- **gate/service.go — real move**: `gateMaterialize`, `runGate` из `core/gate.go` перенесены в `gate.Service` с методами `Materialize` и `Run`. `core/gate.go` — shim `→ gate.Service`. Убраны дубли `truncate`, `basename`, `kindOf` — теперь в gate.
- **pipeline/planner.go — real DAG + цикл**: `PlanPipeline` строит `DAG{Nodes:[id,plugin,phase,bind,after_foreach], Edges:[from,to,via]}` с фазами pre/foreach/post. `validator.go` добавил `DetectCycle()` — DFS с `visited 0/1/2`, находит цикл `a → b → a` и возвращает ошибку `цикл в DAG: ...`. Теперь `Validate` ловит циклы до рана, а `plan` выводит DAG.
- **pipeline/model.go**: `Permissions.Network` теперь `[]NetworkPermission{host,port,any_host,note}` вместо `[]map[string]interface{}` — схема явная.
- **core/* — теперь shim'ы**: `context.go → context.Ctx`, `journal.go → journal.Journal`, `manifest.go → plugin.Engine` (с Cache для тестов), `types.go → pipeline.*`, `validate.go → pipeline.Validate`, `executor.go → plugin.Exec` с совместимостью `ExecResult{Status,TimedOut,shouldRetry}`, `fileref.go — real logic с хинтом КОРНЯ ПРОЕКТА`.
- **Плагины — убран дубляж**: удалены плоские `plugins/csv_loader`, `dir_lister`, `disposable_checker`, `email_triage`, `llm_*`, `syntax_mx_checker`, `text_analyzer`, `my_summarizer` из корня `plugins/`. Теперь только `plugins/official/` (5: disposable_checker, llm_anthropic, llm_gemini, llm_openai, syntax_mx_checker) + `plugins/community/` (5: csv_loader, dir_lister, email_triage, text_analyzer, my_summarizer). Все `pipelines/*.yaml` и `examples/pipelines/*.yaml` обновлены на `plugins/community/*` и `plugins/official/*`. `TestPluginTestShippedPlugins` уже сканирует `plugins/*` и `plugins/*/*`.
- **Артефакты**: удалены `orchestrator-v0.11.zip`, `orchestrator-v0.12.1.zip` из гита, добавлен `*.zip` в `.gitignore`. `var/runs/` только `.gitkeep`.
- **Мелочи**: `execution.Sanitize` теперь реальный (не `return s`), `gate.Materialize` реальный, `cli/validate.go` план выводит DAG и фазу, убран TODO про циклы, `execution/scheduler.go` и `plugin/transport.go` — без «заглушка» комментариев, с реальной логикой.
- **Тесты**: `go test ./... ok`, `csv_foreach` + `csv_foreach_summary` зелёные, цикл `a→b→a` ловится валидатором, resume через `RunStore`.

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
