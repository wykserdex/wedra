# orchestrator v0.24 — человек в гейте из браузера: решение кнопкой в консоли, не только в терминале

Локальный оркестратор цепочек с человеком в петле.

> **Название:** репо — `wedra`, продукт/бинарник — `orchestrator`
> (имя модуля в go.mod и `cmd/orchestrator`; так было с M1). Пока держим оба:
> переименование = имя release-ассетов, install-инструкции и ссылки реестра —
> решаем осознанно, не на бегу. M1–M5 закрыты, M6 GUI в работе (срезы 1–2: консоль + браузерный гейт). Честная версия: **v0.24**.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов, 8+1 пайплайнов, 0 провалов, ядро 8.5–9/10.

> «каждый кусок можно независимо написать, протестировать и заменить» · «контракт честный»

```
orchestrator/
├── VERSION              # 0.21 (читает бинарник, CWD в приоритете)
├── registry.yaml        # реестр: 19 плагинов + 12 пресетов, формат заморожен
├── cmd/
│   ├── orchestrator/    # точка входа (CLI + REST API)
│   └── tool/            # compat-шим M5 (run/validate/plugin/runs)
├── protocol/            # VERSION (0.2), CHANGELOG, v0.2/PROTOCOL.md
├── schemas/             # pipeline.v0.2, manifest, request, response
├── internal/
│   ├── pipeline/        # модель, парсер, валидатор, планер (DAG), when
│   ├── execution/       # runner: foreach-фазы, step-foreach, parallel_group
│   ├── plugin/          # process, transport, enforce, manifest
│   ├── registry/        # реестр v0.1, install, pin-контракт (RefToDir)
│   ├── journal/         # writer/reader + RunStore (filesystem, json)
│   ├── gate/            # human_gate: service, terminal, typing
│   ├── context/         # shared context, dot-пути
│   ├── cli/             # pipeline|plugin|runs|version + install
│   ├── api/             # REST API (M6, отложен)
│   └── core/            # shim-фасад + интеграционные тесты (см. «Судьба core/»)
├── plugins/             # official/ 5, community/ 14
├── examples/            # 16 пайплайнов (демо v0.20: when/foreach/parallel)
├── docs/                # plugin-dev, quickstart, resume, architecture
├── archive/             # устаревшие доки (M5, LANDING, POSTS)
├── web/static/          # GUI scaffold (M6, отложен)
└── var/runs/            # журналы + --resume
```

**Судьба `core/` (решение v0.18):** shim остаётся — это стабильный внутренний
фасад над `execution`/`pipeline`/`journal` (алиасы типов + тонкие обёртки),
API-поверхность для `cmd/*` и будущего M6, там живут интеграционные тесты.
Удалять его = сломать compat-шим `tool` без выгоды; чистка — не раньше M6.

## Быстрый старт (CLI — мясо)

```bash
go build -o orchestrator ./cmd/orchestrator
./orchestrator version   # v0.24
go test ./...            # 112 тестов

# плагины
./orchestrator plugin validate plugins/csv_loader
./orchestrator plugin test plugins/csv_loader   # 7 PASS
./orchestrator plugin test plugins/email_triage # 10 PASS

# пайплайны
./orchestrator pipeline validate examples/email_check.yaml
./orchestrator pipeline lint examples/csv_foreach.yaml
./orchestrator pipeline plan examples/csv_foreach.yaml

# v0.12: foreach по результату шага (было только input.*)
./orchestrator pipeline run examples/csv_foreach.yaml --yes
# фаза 1: load rows → фаза 2: foreach row → check → review, ok=2

# v0.12: --resume
./orchestrator runs list
./orchestrator runs show <run_id>
./orchestrator pipeline run examples/email_triage_chain.yaml --yes --resume=<run_id>
./orchestrator runs resume <run_id> examples/email_triage_chain.yaml --yes

# совместимость tool
./tool run examples/csv_foreach.yaml --yes
./tool runs list
```

## v0.16: install-путь («взял и использовал»)

Плагин или пресет — не из локального каталога, а из реестра (`registry.yaml`,
включён в это репо):

```bash
# пресет из реестра + АВТОУСТАНОВКА его плагинов в plugins/
./orchestrator pipeline install email_check
./orchestrator pipeline run examples/email_check.yaml --yes

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

## Что нового в v0.24 (человек в гейте из браузера)

GUI-срез 2: human_gate больше не «только из терминала». Запустил ран из
консоли без --yes — он блокируется на гейт-шаге, в деталке появляется
**гейт-карточка**: поля формы (текущие значения, редактируемые — с
предзаполнением), кнопки действий из манифеста. Решение уходит в живой ран,
ран продолжается. Терминальный ввод (CLI) — как был, шов `GateUI` (v0.15)
наконец подтянут до жизни.

- **API**: `POST /api/run {file, yes:false}` — запуск с браузерными гейтами
  (ID рана известен до старта, в ответе 202); `POST /api/runs/<id>/gate
  {action, edits}` — решение (409: гейта нет / уже решён / ран мёртв);
  `GET /api/runs/<id>/gate` — ожидающий ли гейт (для рендера карточки).
- **Контракт гейта в журнале**: новое событие `gate_wait` (step, form с
  текущими значениями, actions) до блокировки; `gate_retry` — мусорное
  решение с причиной; `gate_decision` — как в v0.23 (+`skipped_edits` при
  правках с неверным типом). Все те же правила: 5 мусорных → стоп, EOF →
  стоп, **никогда — молчаливый accept**.
- **Механика**: `ChannelUI` (канал, CAS против двойного submit, терминальный
  EOF) + `StructuredUI` — опциональный интерфейс поверх шва `GateUI`;
  рантайм прокидывает фабрику ввода (`RunOptions.GateUI`), runID генерируется
  заранее. По пути пойман и задокументирован баг: `core.Run` копировал
  опции поле-в-поле и молча терпал новые — теперь прокидывает целиком.
- **Фронт**: гейт-карточка (рендер один раз на `gate_wait` — не затирает
  вводимое), правки как JSON (пусто = оставить), статус «отправлено».
  Таймлайн знает `gate_wait`/`gate_retry`.
- **Обещание из прошлого хода**: ссылка на мёртвый прототип `/editor/`
  убрана из консоли (переписываем редактором позже, не на глаз).
- New example: `gate_demo.yaml` (минимальный гейт без плагинов — и в CI, и
  как «посмотри, как работает» для новичка).

Тесты: +7 (ChannelUI: accept+правки, reject, EOF, 5 мусорных, тип-скап,
race-контур Send/Close; API: полный цикл wait→decision→ok, reject→aborted,
--yes без pending, busy-409). CI: новый live-шаг «v0.24 browser gate».

## 

Реальный баг-репорт (проверен живьём) — все пункты закрыты:

- **Критично: контракт рантайма теперь проверяет типы и форматы**, а не только
  наличие. До v0.23 `EnforceOutput` принимал `{"total": "НЕ ЧИСЛО"}` при
  `type: number` — «контракт обещает downstream данные заявленного типа», но
  неправильный тип тихо утекал. Теперь:
  - **вывод** плагина сверяется с манифестом (тип + формат email/url/ip) —
    несовпадение = нарушение контракта, шаг падает (on_error применяется);
  - **вход** проверяется симметрично (buildInput) — сломанный upstream больше
    не пролезает в следующий плагин;
  - фикстура `type_drifter` (обещает string, возвращает 42) теперь реально
    ловится рантаймом — интеграционные тесты + unit-тесты контракта.
- **Критично: мёртвая папка `pipelines/`** — `pipeline install` писал пресеты
  в `pipelines/` после переезда на `examples/` (v0.21). Исправлено на
  `examples/`. (gui.go на `examples/` был с v0.22 — проверял старую сборку.)
- **Гейт: EOF/мусорный ввод больше не = авто-accept.** Пустой/нераспознанный
  ответ → переспрос (до 5), EOF или 5 мусорных попыток → **стоп рана**
  (`gate_decision: stop`, reason в журнале). «Человек посмотрит и подтвердит»
  — случайный Enter или Ctrl+D больше не одобряют.
- **Надёжность журнала:**
  - `Snapshot` (context.json) — атомарно (temp+rename): краш в середине
    больше не даёт битый файл для resume;
  - `Event` не мутирует переданный map (footgun) и не глотает ошибки записи
    (disk-full = счётчик + сообщение, финальный отчёт в Close);
  - таймаут плагина убивает **процесс-группу** (Setpgid + kill в момент
    таймаута): python-плагин с дочерними больше не оставляет сирот и не
    держит пайпы рана «замёрзшим» до их естественной смерти;
  - stdout/stderr плагина — с лимитом (16МБ/1МБ): гигантский вывод = честная
    `protocol_violation`, не вся память процесса.
  - (Попутно пойман нюанс Go: embedded `bytes.Buffer` в io-обёртке,
  подставленной в `cmd.Stdout`, заполняется минуя метод-обёртку в exec-пути —
  лимит молча не работал. Частное поле + свой Write — работает, задокументировано
  в коде.)

Тесты: +3 фикстуры (num_only, chatter, spawner), +15 тестов (unit контракта,
гейт с фейк-вводом, журнал, spawn: лимит + group kill).

## 

`orchestrator gui` больше не «отложен, косметика» — это рабочая консоль
(web/static, без внешних зависимостей, офлайн):

- **Раны** — список с живыми статусами (ok / aborted / failed / идёт…),
  автообновление; деталка: **таймлайн** по журналу (шаги, элементы foreach,
  параллельные группы, гейты, skip с причиной) + **контекст** (input.*/steps.*)
  + **live-терминал** (хвост journal.jsonl, авто-скролл, polling 2 c).
- **Запуск из браузера** — `POST /api/run {file, yes}`: in-process ран в
  сервере, один за раз, только `--yes` (человеческий гейт без терминала —
  честно ограничен; для гейта с правкой человека — CLI).
- **Пайплайны** — список, YAML, **DAG** (SVG): фазы pre/foreach/post,
  параллельные группы рамкой, метки `when:`/`foreach:`, рёбра от bind/form/
  when/foreach-путей.
- API: `/api/runs` обогащён (status/steps/started/last),
  `/api/runs/<id>/journal?since=N` (live-хвост), `/api/plan/pipeline`
  аннотирует when/foreach/parallel_group.
- Баг, пойманный при этом: `/api/runs` смотрел только индекс runs.db и
  скрывал свежие filesystem-раны — теперь FS-директории = источник правды,
  runs.db только дополняет.

Старый scaffold-редактор (drag-and-drop) сохранён в `/editor/` как прототип.
Человеческий гейт из браузера (submit правок в живой ран) — следующий срез.

## 

Реструктуризация по согласованному дереву — `git mv`, ноль логики:

- **`protocol/`** — единый дом протокола: `v0.2/PROTOCOL.md` (контракт),
  `CHANGELOG` (история версий, слита из versions/), `VERSION` (0.2).
- **`schemas/`** — единое место схем: `pipeline.v0.2`, `manifest`, `request`,
  `response` (были в двух местах: protocol/schemas и schemas/pipeline).
- **`pipelines/` → `examples/`** — плоское, 16 пайплайнов; старые M5-дубли
  в examples/{pipelines,plugins} вычищены (идентичные копии).
- **`docs/`** — `plugin-dev.md` (экс-TUTORIAL_PLUGINS), `quickstart.md`
  (экс-START_HERE), `resume.md` и `architecture.md` (новые).
- **`archive/`** — LANDING/POSTS/M5_FEEDBACK/TESTER_PACKET_5.
- **trust-гейт дожат**: `registry validate --local-source` теперь требует,
  чтобы путь записи резолвился в локальном source (ранее при расхождении
  резолвинга тихо уходил в git-клон — молчаливый слепой участок).
- Все ссылки перетянуты: CI, README, CONTRIBUTING, demo.sh, docs, код
  (help-тексты, скелет `plugin create`).

## Что нового в v0.20 (управляющий поток)

Контрольный поток переехал на уровень шага — три механизма (PROTOCOL §12):

- **`when:`** — условие шага: строка (путь, «истинно?») или
  `{path, op, value}` с операторами `eq/neq/gt/gte/lt/lte/exists/missing/contains`.
  Ложно → шаг `skipped` (журнал `step_skipped`, reason=when).
- **`foreach:` на шаге** — шаг по каждому элементу массива
  (`input.*` или `steps.<id>.<field>`); `steps.<id>_all` — агрегат,
  `steps.<id>` — последняя итерация. stop = стоп рана (здесь нет «элемента»).
- **`parallel_group`** — смежные шаги с одинаковой группой исполняются
  параллельно, барьер ждёт все ветки; слияние выходов в порядке списка
  (детерминизм). human_gate в группах запрещён (гейты сериализуют терминал).
- Diamond-паттерн (A,B → C) — из коробки; **циклы — сознательно не
  поддерживаются**: цикл принадлежит плагину (бюджет итераций держит он),
  ядро блокирует циклы в графе (решение в PROTOCOL §10).

Демо в `examples/`: `when_demo`, `foreach_step_demo`, `parallel_demo`
(все в CI + в реестре как пресеты). Планер (`pipeline plan`) аннотирует
DAG: when/foreach/parallel_group + рёбра зависимостей.

## Что нового в v0.19 (волна 2, батч 2)

Ещё два community-автора — 6 плагинов. На этот раз код прислали, вшитый в
веб-шоукасы (React/Vite): источники вытащены из TS-данных, каждый прогнан
через конформность:

- `iban_validator` — IBAN (ISO 13616): синтаксис, страна, длина, контрольная
  сумма mod-97 (7 тестов);
- `phone_normalizer` — список телефонов → E.164, разбор валид/инвалид,
  транк '8'→'7' для RU (7);
- `text_similarity` — Левенштейн + нормализованное сходство + флаг
  near-duplicate (7);
- `date_parser` — человекочитаемые даты → ISO 8601 (5);
- `json_flatten` — JSON → плоский map, точки в ключах (4);
- `phone_check` — одиночный телефон → E.164 (5);
- 3 пресета с `human_gate`: `iban_check`, `phones_audit`, `near_dupe_check`
  (live-прогон `--yes` зелёный).

Правки при приёмке (7, все в тестах/guard'ах, логика не тронута): 4 устаревших
expect `bad_input` → `platform:bad_input` (exit≥2 сохраняет код как
`platform:<code>`), valid-флаг в ожидании, 2 guard-типа — доменная ошибка
(exit 1) заменена платформенной (exit 2). Конфликт имён: оба автора написали
`phone_normalizer` — батч-вариант сохранил имя, одиночный переименован в
`phone_check`. Реестр: 19 плагинов + 9 пресетов.

## Что нового в v0.18 (волна 2)

Первые **community-плагины в публичном реестре** — тестер №1 (M5) написал 5
плагинов и вернул оригинал `dir_lister` (вместо моей реконструкции — открытый
вопрос M5 закрыт): `word_freq`, `json_diff`, `batch_email_triage`,
`report_formatter`, `dir_lister`. Каждый принят через конформность
(`registry validate`), атрибуция в манифесте, у всех `permissions` честные
(без сети и ключей).

Кто угодно: `orchestrator plugin install word_freq` — и плагин у вас в `plugins/`.
Свой плагин — `docs/plugin-dev.md` (15 минут) + `OUTREACH_ROUND2.md` (меню идей).

## Что нового в v0.17 (trust)

- **`orchestrator registry validate [--registry=<src>] [--local-source=<dir>]`** —
  trust-гейт реестра: для КАЖДОЙ записи — манифест, `id` = имя в реестре,
  `plugin.test.yaml` с зелёными тестами (конформность обязательна для реестра),
  пресеты — парсинг + валидация. Запись без конформности = не запись, а долг.
- **CI** (`.github/workflows/ci.yml`): `registry validate` на каждом PR (local
  source) + отдельный job на теге — по **реальным** пинам (git-клоны `version`).
- **declare-now (сеть)**: `network: deny` в пайплайне + плагин, заявивший
  `permissions.network` — **ошибка до любого эффекта**. Каждый subprocess получает
  `WEDRA_NETWORK=allow|deny`; заявленная сеть пишется в журнал
  (`step_start.network_declared`) — аудит в `runs show`.
- **Кросс-проверка secrets**: `secrets:` пайплайна ↔ `permissions.secrets`
  манифестов — warning в обе стороны (осиротевший ключ / необъявленный).
- **CONTRIBUTING.md** — чек-лист попадания в реестр (плагин/пресет/ревьюер).
- PROTOCOL.md §11 — `permissions: declare-now` (L1: контракт + аудит, не песочница).
- Тесты: **113 PASS**.

## Что нового в v0.16 (install-путь)

- **Реестр** `registry.yaml` (v0.1, формат заморожен) — в корне репо: `plugins` +
  `presets`, `source`/`path`/`version`/`description`. Один репо = реестр +
  источник; позже реестр выносится в отдельный репо — формат не меняется.
- **`plugin install <name>[@version]`** — clone из `source`, валидация,
  lock-файл `.wedra` (name/source/version) в каталоге плагина.
- **`pipeline install <name|file|url>`** — пресет → `examples/<name>.yaml` +
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
- Демо: `examples/csv_foreach.yaml` — `load` (csv_loader) → `foreach: steps.load.rows` → `check` (text_analyzer на `input.row.name`) → `review` (gate)

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
- [x] v0.17: **trust** — `registry validate` как CI-гейт, declare-now сеть (`network: deny` + `WEDRA_NETWORK` + аудит), кросс-проверка secrets, CONTRIBUTING
- [x] v0.18: **волна 2** — 5 community-плагинов (тестер №1) в реестре, оригинал dir_lister вместо реконструкции, fix версии бинарника
- [x] v0.19: **волна 2, батч 2** — 6 community-плагинов (два новых автора) + 3 пресета с human_gate; конфликт имён решён (phone_check)
- [x] v0.20: **управляющий поток** — `when:`, `foreach:` на шаге, `parallel_group` (PROTOCOL §12, 3 демо в CI)
- [x] v0.21: **хирургия** — protocol/, schemas/, examples/, docs/, archive/; trust-гейт: локальный путь обязателен
- [x] v0.22: **GUI-консоль** — live-терминал, таймлайн, DAG, запуск --yes из браузера (`orchestrator gui`)
- [x] v0.23: **контракт рантайма** — типы/форматы на входе и выходе, гейт без молчаливого accept, атомарный журнал, group kill, лимиты вывода
- [x] v0.24: **человек в гейте из браузера** — гейт-карточка в консоли, решение в живой ран (API + `gate_wait`/`gate_retry`), ссылка на мёртвый /editor/ убрана
- [ ] v1.0: GUI full (редактор, import YAML) + маркетплейс v1 (гейт из браузера — уже в v0.24)

## Тесты

`go test ./...` — 156 тестов PASS, `csv_foreach` зелёный (ok=2), resume — все элементы уже пройдены, install- и trust-сценарии покрыты e2e.

## Версионирование

- v0.9 (ex v9), v0.9.1 (ex v9.1), v0.10 (ex v10), v0.11 (GUI scaffold), v0.12 (CLI focus), v0.13 (честный перенос), v0.14 (JsonStore — тогда ещё назывался SQLiteStore, в v0.15 переименован честно), v0.15 (честный релиз), v0.16 (install-путь), v0.17 (trust), v0.18 (волна 2: community-плагины), v0.18.1 (долги), v0.19 (волна 2, батч 2), v0.20 (управляющий поток), v0.21 (хирургия структуры), v0.22 (GUI-консоль), v0.23 (контракт рантайма), v0.24 (браузерный гейт)
- Дальше: редактор (переписать: drag по сетке, палитра, undo) + решение по naming (orchestrator → wedra?) → v1.0 — GUI + маркетплейс
- **Схема букв (с v0.23, договорённости):** цифра = функциональный срез;
  буква = фикс-релиз внутри среза без новых фич (v0.24a, v0.24b…).
  Отпущенный tag больше не сдвигается. После `z` — срез был объявлен рано,
  поднимаем цифру. (В v0.23 я два раза force-moved тег под race- и
  кросс-сборочные фиксы — с v0.24 так не будет: фикс = v0.23a-стиль.)
