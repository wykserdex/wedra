# Changelog — честная 0.x

## v0.22 (2026-08-30) — GUI-консоль (первый срез)

- `orchestrator gui` — рабочая консоль (было «отложено, косметика»):
  web/static без внешних зависимостей (офлайн: inline CSS/JS).
- **Live-терминал**: хвост journal.jsonl с авто-скроллом (polling `?since=N`),
  параллельно — таймлайн (шаги/элементы foreach/параллельные группы/гейты)
  и снапшот контекста (input.*, steps.*).
- **Запуск из браузера**: `POST /api/run {file, yes}` — in-process ран в
  сервере (один за раз, только --yes; гейт с правкой человека — из CLI).
- **DAG** (SVG): фазы, parallel_group-рамки, метки when/foreach, рёбра от
  bind/form/when/foreach-путей. `/api/plan/pipeline` аннотирует when/
  foreach/foreach_item/parallel_group в нодах.
- API: `/api/runs` — status/steps/started/last по журналу;
  `/api/runs/<id>/journal?since=N` — live-хвост.
- **Баг (найден при внедрении)**: `/api/runs` резолвил только runs.db-индекс и
  не показывал свежие filesystem-раны — теперь FS-директории = источник
  правды, runs.db дополняет (artifacts, старые индексы).
- Баг: статус aborted читал int, а JSON отдаёт float64 — email_check-ран с
  aborted=1 показывался «ok».
- Старый scaffold (drag-and-drop редактор) → /editor/ (прототип, без изменений).
- CI: новый step «v0.22 gui api live» (health + POST /api/run + ожидание ok).


## v0.21 (2026-08-30) — хирургия структуры

- **`protocol/`** — единый дом: `v0.2/PROTOCOL.md`, `CHANGELOG` (из
  versions/{v0.1,v0.2}.md), `VERSION` (0.2). Заголовок PROTOCOL.md поправлен
  (v0.1 → v0.2), устаревшая строчка §9.6 про foreach — актуализирована.
- **`schemas/`** — pipeline.v0.2, manifest, request, response в одном месте
  (были разнесены: protocol/schemas/v0.2 + schemas/pipeline).
- **`pipelines/` → `examples/`** (16 пайплайнов, плоское); старые M5-дубли
  examples/{pipelines,plugins} удалены (слово в слово с актуальными).
- **`docs/`** — plugin-dev.md (экс-TUTORIAL_PLUGINS.md), quickstart.md
  (экс-START_HERE.md), resume.md, architecture.md (новые).
- **`archive/`** — LANDING, POSTS, M5_FEEDBACK, TESTER_PACKET_5.
- **trust-гейт (реальный баг, найденный при переезде)**: `registry validate
  --local-source` резолвил пресеты по имени через клон, минуя локальный путь —
  запись с «висячим» path проходила зелёной. Теперь sameRepo-ветка
  pluginSourceDir статит путь: не найден в локальном source = явная ошибка.
- Ссылки перетянуты: CI (glob examples/, paths), README, CONTRIBUTING,
  demo.sh, docs, help-тексты CLI, скелет `plugin create`.
- Логики ядра не тронута (кроме stat-проверки выше); все тесты зелёные.


## v0.20 (2026-08-30) — управляющий поток на уровне шага

- **`when:`** — условие выполнения шага (PROTOCOL §12.1). Строковый формат
  (путь, «истинно?») или `{path, op, value}`; операторы `truthy/exists/missing/
  eq/neq/gt/gte/lt/lte/contains`. Честные числа (YAML-int = JSON-float64),
  null-семантика для missing. Ложь → `skipped` (журнал step_skipped, reason=when);
  не оценивается (gt по строке) → ошибка рана.
- **`foreach:` на шаге** (§12.2) — шаг по каждому элементу массива из
  `input.*` или `steps.<id>.<field>`; переменная элемента — `input.<foreach_item>`;
  `steps.<id>` = последняя итерация, `steps.<id>_all` = агрегат (skip-итерации
  не входят). В отличие от pipeline-foreach, stop останавливает весь ран.
  Валидатор: источник — только из input или уже выполненных шагов.
- **`parallel_group`** (§12.3) — смежные шаги с одинаковым именем группы
  исполняются параллельно (ветки на копиях контекста, барьер, детерминированное
  слияние в порядке списка). stop/платформенная ошибка в ветке = стоп рана;
  human_gate в группе запрещён (сериализация терминала). Журнал:
  parallel_start/parallel_end + переплетённые события веток.
- Правка семантики `on_error=skip`: шаг больше не оставляет **чужое** значение
  в неймспейсе (чистит `steps.<id>`) — иначе значение предыдущей итерации
  утёкало бы в foreach-агрегаты.
- Валидатор v0.20: правила when/foreach/parallel_group (операторы, пути,
  смежность групп, конфликты foreach×after_foreach×group, гейты вне групп) +
  динамический `input.<foreach_item>` как легальный источник порта.
- Схема `schemas/pipeline/v0.2.schema.json`: when/foreach/foreach_item/
  parallel_group (+ `after_foreach`, которого не хватало с v0.12).
- Планер: DAG-ноды несут when/foreach/parallel_group, рёбра от путей
  when/foreach; фаза `parallel`.
- Демо: `pipelines/when_demo.yaml`, `foreach_step_demo.yaml`,
  `parallel_demo.yaml` (live в CI + реестр-пресеты).
- Тесты: unit (when: 6 групп сценариев) + интеграция (when true/false/numeric,
  step-foreach агрегат/stop/skip, parallel группа/stop/when-skip, правила
  валидатора) — 12 новых тестов.


## v0.19 (2026-08-30) — волна 2, батч 2: 6 community-плагинов + 3 пресета

- **6 плагинов от двух новых community-авторов** (присланы как React-веб
  шоукасы; источники извлечены из TS-данных, конформность по каждому):
  - `iban_validator` (7 тестов), `phone_normalizer` (7), `text_similarity` (7)
    — «community · волна 3 (plugin-pack wedra)»;
  - `date_parser` (5), `json_flatten` (4), `phone_check` (5) — «community ·
    wedra showcase».
- **Конфликт имён**: оба автора написали `phone_normalizer` (разбивка меню не
  спасла). Батч-вариант (список → E.164 + разбор валид/инвалид) сохранил имя;
  одиночный переименован в `phone_check`.
- 3 пресета с `human_gate`: `iban_check`, `phones_audit`, `near_dupe_check`
  (live-прогон `--yes` зелёный; ссылки на плагины — локальные пути, как у
  штатных пресетов).
- Правки при приёмке (7, только в тестах/guard'ах, логика не тронута):
  4 устаревших expect `bad_input` → `platform:bad_input` (в v9 exit≥2
  сохраняет код как `platform:<code>`); valid-флаг в ожидании
  phone_normalizer; 2 guard-типа возвращали доменную ошибку (exit 1) вместо
  платформенной (exit 2) — исправлено в json_flatten и phone_check.
- Реестр: 19 плагинов + 9 пресетов = 28 записей, все @ v0.19,
  `registry validate --local-source` зелёный.


## v0.18.1 (2026-08-30) — долги

- **Схема догнала контракт**: `schemas/pipeline/v0.2.schema.json` теперь знает
  `secrets` (v0.16), `network` (v0.17) и `foreach steps.*` (v0.12 — паттерн
  поправлял `^input\.` → `^(input|steps)\.`).
- **`my_summarizer`** — tutorial-skeleton с M5 (`author: TODO`, `description: TODO`)
  убран из репо и реестра (в git-истории остаётся; name мешал в community/).
- **Судьба `core/` записана** (README): shim остаётся как стабильный фасад для
  `cmd/*`/M6; чистка не раньше M6.
- **LANDING.md / POSTS.md** помечены устаревшим (M5-цифры), призыв заменён на волну 2;
  POSTS чек-лист: токен — актуален, zip-релиз — устарел (auto-release на тег).
- Реестр: 13 плагинов + 6 пресетов.

## v0.18 (2026-08-30) — волна 2: первые community-плагины в реестре

- **5 плагинов тестера №1 в реестре** (волна 2, «напиши плагин — попади в реестр»):
  - `word_freq` — топ-N частых слов (6 конформных тестов);
  - `json_diff` — diff двух JSON: added/removed/changed (4);
  - `batch_email_triage` — батч-триаж email: синтаксис + disposable, агрегат по вердиктам (4);
  - `report_formatter` — агрегат batch_email_triage → текстовый отчёт (2);
  - `dir_lister` — **оригинал тестера №1 заменит мою реконструкцию** (открытый вопрос M5 закрыт).
- Правки при приёмке (3, все в тестах/манифесте, логика не тронута):
  `word_freq` — `format` на `type: number` убран; `batch_email_triage` — устаревший
  expect `crash` → `platform:bad_input`; `dir_lister` — фикстуры `fixtures/sample_dir`
  потерялись в зипе — восстановлены.
- **fix(version)**: `api.Version` был const 0.14.1 — бинарник из Release показывал
  старую версию (файл VERSION рядом с бинарником отсутствует). Теперь `var` +
  `ldflags -X` из тега сборки; `tool --help` с захардкоженным v0.12 — тоже.
- Атрибуция: `author: "community · тестер №1 (волна 2, v0.18)"` в манифестах.
- Тесты: 112 PASS (ядерные); конформность: 14 плагинов, 67 тестов — CI.

## v0.17 (2026-08-30) — trust: реестр проверяется, сеть по декларации

- **`orchestrator registry validate [--registry=<url|path>] [--local-source=<dir>]`** — trust-гейт реестра: манифест каждой записи, `id` = имя в реестре, `plugin.test.yaml` с зелёными конформными тестами; пресеты — парсинг + валидация. Exit 1 при любом провале.
- **CI**: `registry validate` на каждом PR (local source) + job `registry-release` на теге — проверка по реальным git-пинам (`version` из registry.yaml).
- **declare-now (сеть)**: `network: deny` в пайплайне + плагин с `permissions.network` — ошибка до любого эффекта (валидатор и раннер независимо). Subprocess получает `WEDRA_NETWORK=allow|deny`; заявленная сеть — в журнал (`step_start.network_declared`), видна в `runs show`.
- **Кросс-проверка secrets**: `secrets:` пайплайна ↔ `permissions.secrets` манифестов — warning в обе стороны.
- **CONTRIBUTING.md** — чек-листы: плагин в реестр, пресет в реестр, ревьюер.
- **PROTOCOL.md §11** — `permissions: declare-now`: L1 = контракт + аудит (песочница сознательно вне уровня).
- Фикстуры: `net_demo` (заявляет сеть + секрет), `net_probe` (читает `WEDRA_NETWORK`).
- **CI-долг закрыт**: workflow краснел с v0.14 — `tool pipeline ...` и `tool runs` (с флагами)
  никогда не существовали в `tool` (только в `orchestrator`), а `tool plugin list/search`
  добавлены (общий сканер `core.ScanPlugins`). Workflow переписан на `./orchestrator`
  для pipeline/runs-шагов.
- **Release-бинарники**: каждый тег теперь — GitHub Release (orchestrator + tool,
  linux/darwin/windows × amd64/arm64) — то, что получают авторы во «волне 2».
- **Волна 2 (аутрич)**: `OUTREACH_ROUND2.md` — «напиши плагин, попади в реестр»:
  персональные сообщения (тёплые M5-авторы + холодные), меню идей без сети/ключей,
  3 способа сдачи (zip/PR/свой репо с тегом). Туториал: §2 permissions, §7 — сдача в реестр.
- Тесты: 112 PASS (+5: network deny validate/run, WEDRA_NETWORK env, secrets cross, load local file).

## v0.16 (2026-08-30) — install-путь: «взял и использовал»

- **`registry.yaml` в корне репо** — реестр v0.1 (формат заморожен, как протокол): секции `plugins` + `presets`, поля `source`/`path`/`version`/`description`. Минимальная конфигурация: один репо = реестр + источник. Реестр можно вынести в отдельный репо — формат не меняется.
- **`orchestrator plugin install <name>[@version] [--registry=<url|path>] [--dest=plugins]`** — установка из реестра в `plugins/<name>`: git clone depth-1, валидация манифеста, lock-файл `.wedra` (name/source/version).
- **Реестровые ссылки на плагины**: в `plugin:` теперь можно голое имя (`csv_loader`) и pin (`csv_loader@v0.16`) — вдобавок к локальным путям (они работают как раньше). Пин-конфликт (установленная ≠ требуемая версия) — явная ошибка с подсказкой.
- **`orchestrator pipeline install <name|file.yaml|url> [--registry=...]`** — пресет из реестра/файла/URL → `pipelines/<name>.yaml` + **автоустановка недостающих плагинов** (переустановка под пин при несовпадении версий, ошибка при конфликте версий в одном пайплайне) + валидация совместимости.
- **`secrets: [KEY]` в пайплайне** — имена переменных окружения, которые обязаны быть заданы. Validate — предупреждение, run — жёсткая ошибка до любого эффекта. Значения в YAML не живут — только имена.
- **Оффлайн**: локальный реестр (каталог с `registry.yaml`) и локальный source (путь) — без сети.
- Тесты: 107 PASS (+5: контракт registry, secrets).


## v0.15 (2026-08-30) — честный релиз: названия совпадают с кодом

- **journal/store.go: `SQLiteStore` → `JsonStore`** — честное имя. Это никогда не был SQLite (зависимости на SQLite-драйвер не тянулось — см. поправку к v0.14 ниже). Реализация: pure Go JSON-файл `var/runs/runs.db`, полный rewrite на каждый append, рассчитан на single writer. Журнал (`journal.jsonl`) остаётся единственным источником истины, store — вторичный индекс.
- **`loadDB` больше не глотает ошибки**: битый индекс = явная ошибка при записи (файл не перезаписывается пустым реестром); при чтении деградирует в FS. Было: `db, _ := loadDB()` → тихая потеря всех зарегистрированных прогонов при следующем `saveDB`.
- **`dbRun`: убраны мёртвые поля** `pipeline`, `created_at` (не заполнялись нигде; имя pipeline — в событии `run_start` журнала).
- **CLI: `--store=sqlite` → `--store=json`** (`runs list/show`, `pipeline run`, ci.yml, help).
- **`common.Truncate` — rune-safe**: было `s[:n]` — могло срезать UTF-8 символ пополам (мусор в русских текстах ошибок). Убраны локальные копии из `plugin/process.go`, `gate/service.go`, `execution/runner.go` — одна функция в common + тест.
- **`core.Engine`, `plugin.Engine`: убран двойной кэш** (`Cache` + `cache` велись параллельно) — один `Cache`.
- **gate: `GateUI`-интерфейс + `StdinUI`** — шов для GUI/API (M6): канал ввода human_gate заменяется без изменения `Service` (`NewServiceWithUI`).
- **Версии синхронизированы**: VERSION / README / CHANGELOG / теги говорят одну историю (v0.15).
- Тесты: `go test ./...` — 102 PASS (добавлен тест rune-безопасности Truncate).

## v0.14.1 (2026-08-28) — добить заглушки после фидбека типов

Типы заценили v0.14, но попросили допилить заглушки:

- **plugin/transport.go — real**: `StdioTransport.Invoke` теперь реальный — вызывает `Exec(manifest, input, timeout)` и возвращает `json.Marshal(output)`, ошибка если `!OK()`. Добавлены `NewEnvelope`, `Marshal`, `ParseEnvelope` с проверкой `protocol_version 0.2/0.3`. Убраны `return nil, nil` заглушки.
- **execution/scheduler.go — real DAG**: `BuildGraphFromPipeline` строит `Nodes` + `Edges` по `bind` и `form` зависимостям (`steps.<id>` и `steps.<id>_all`). `TopoSort()` — Kahn, ловит циклы. `IndependentBatches()` — батчи независимых шагов для параллели (уровни по зависимостям). `BuildGraph()` остался для совместимости, теперь не пустой.
- **api/server.go — SQLite support**: `handleRuns` и `handleRunDetail` теперь проверяют `var/runs/runs.db` — если есть, используют `SQLiteStore.ListRuns/ListArtifacts`, иначе FS. Отдают `store: sqlite/fs` + `artifacts` список.
- **execution/runner.go**: `runWithStore` теперь использует `store.Create(runID)` для новых ранов, а не `journal.NewJournal` напрямую — чтобы SQLite DB получала запись в `runs` таблицу + `run_start` event. Убран неиспользуемый `filepath` импорт.
- **journal/store.go**: SQLiteStore теперь pure Go JSON DB (без CGO/modernc) — файл `runs.db` с `runs/events/artifacts`, работает без внешних зависимостей, `go test` зелёный без `modernc`.
- **Тесты**: `go test ./... ok`, `plugin list` 10, `pipeline lint` ловит file_ref, sqlite live с `runs.db` 409 байт → 2 рана.

## v0.14 (2026-08-28) — JSON RunStore, lint file_ref error, plugin search, conformance в CI

- **journal/store.go — RunStore** *(поправка v0.15)*: `SQLiteStore` (переименован в `JsonStore` в v0.15) — индекс прогонов в одном JSON-файле `var/runs/runs.db` (секции `runs`/`events`/`artifacts`), полный rewrite на каждый append, pure Go без внешних зависимостей. Методы: `Create` пишет в FS + индекс runs, `AppendEvent` — FS + events (с item_index), `SaveArtifact` — FS + artifacts, `MaxItemIndex` — максимум `item_index` по `item_end` с fallback в FS, `ListRuns`/`ListArtifacts` — из индекса с fallback в FS, `Close()`. **Поправка (v0.15): исходный текст пункта описывал использование `modernc.org/sqlite` и SQL-схему — в коде этой зависимости не было никогда, реализацией был JSON-файл; пункт переписан по факту.** CLI `--store=fs|json --db-path=var/runs/runs.db`.
- **execution/runner.go**: `RunOptions{Store, DBPath}`, `Run` выбирает `FilesystemStore` или `JsonStore`, `runWithStore` принимает `journal.RunStore` интерфейс (а не конкретный FS). `core/runner.go` shim прокидывает Store/DBPath.
- **pipeline/validator.go — Lint file_ref error**: новый `Lint(pf, eng, projectRoot)` — вызывает `Validate` + проверяет file_ref литералы из `input.*`: ищет файл от плагина (`<plugin_dir>/<path>`) и от корня проекта, если не найден ни там ни там → error `file_ref: файл не найден ни от плагина ни от корня`. Если найден только от корня → warning с хинтом. `cli/validate.go` теперь различает `validate` (только Validate) и `lint` (Lint с file_ref error). `pipeline lint` → `OK: lint пройден (включая file_ref)` или ошибка.
- **plugin search**: `orchestrator plugin search <query>` и `plugin list` — сканирует `plugins/official/*` + `plugins/community/*`, ищет по `id + description + author` (case-insensitive). Выводит `id version dir description`.
- **conformance в CI**: `ci.yml` теперь гоняет `go test -run TestConformance`, `plugin list/search`, `pipeline lint/plan`, `foreach+after_foreach live`, `resume live`, `json store live` (run + list + show), `schemas check`.
- **runs CLI**: `runs list/show` поддерживают `--store=json --db-path=...` и показывают artifacts count + список artifacts. `ListArtifacts`, `LoadArtifact`, `ListRuns` в `RunStore` интерфейсе, реализованы в FS и JsonStore.
- **Тесты**: `go test ./... ok` (core 93 теста на тот момент), `plugin list` 10 плагинов, `lint` ловит отсутствующий file_ref, json store live проверен прогоном.

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
