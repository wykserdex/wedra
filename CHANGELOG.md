# Changelog — честная 0.x

## v0.14 (2026-08-28) — SQLite real, lint file_ref error, plugin search, conformance в CI

- **journal/store.go — SQLite real**: `SQLiteStore` теперь реальный, не заготовка. Использует `modernc.org/sqlite` (pure Go, без CGO). Схема: `runs(id, pipeline, created_at)`, `events(id, run_id, type, ts, data, item_index)`, `artifacts(run_id, name, path)`. Методы: `Create` пишет в FS + INSERT в runs, `AppendEvent` — FS + INSERT в events (с item_index), `SaveArtifact` — FS + INSERT в artifacts, `MaxItemIndex` — `SELECT MAX(item_index) FROM events WHERE type='item_end'`, `ListRuns` — из DB, `ListArtifacts` — из DB с fallback в FS. Поддерживается `Close()`. CLI `--store=fs|sqlite --db-path=var/runs/runs.db`.
- **execution/runner.go**: `RunOptions{Store, DBPath}`, `Run` выбирает `FilesystemStore` или `SQLiteStore`, `runWithStore` принимает `journal.RunStore` интерфейс (а не конкретный FS). `core/runner.go` shim прокидывает Store/DBPath.
- **pipeline/validator.go — Lint file_ref error**: новый `Lint(pf, eng, projectRoot)` — вызывает `Validate` + проверяет file_ref литералы из `input.*`: ищет файл от плагина (`<plugin_dir>/<path>`) и от корня проекта, если не найден ни там ни там → error `file_ref: файл не найден ни от плагина ни от корня`. Если найден только от корня → warning с хинтом. `cli/validate.go` теперь различает `validate` (только Validate) и `lint` (Lint с file_ref error). `pipeline lint` → `OK: lint пройден (включая file_ref)` или ошибка.
- **plugin search**: `orchestrator plugin search <query>` и `plugin list` — сканирует `plugins/official/*` + `plugins/community/*`, ищет по `id + description + author` (case-insensitive). Выводит `id version dir description`.
- **conformance в CI**: `ci.yml` теперь гоняет `go test -run TestConformance`, `plugin list/search`, `pipeline lint/plan`, `foreach+after_foreach live`, `resume live`, `sqlite store live` (run + list + show с sqlite), `schemas check`. Go 1.22 с toolchain 1.25 для modernc.
- **runs CLI**: `runs list/show` поддерживают `--store=sqlite --db-path=...` и показывают artifacts count + список artifacts. `ListArtifacts`, `LoadArtifact`, `ListRuns` в `RunStore` интерфейсе, реализованы в FS и SQLite.
- **Тесты**: `go test ./... ok` (core 93 теста), `plugin list` 10 плагинов, `lint` ловит отсутствующий file_ref, sqlite live `32K runs.db` с 8 ранами.

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
