![WEDRA](banner.png)

# WEDRA v0.8 — v1.0-трек: desktop-exe — двойной клик = окно с GUI (WebView2, без CGO)


Локальный оркестратор цепочек с человеком в петле.

> **Имя (v0.26):** продукт — **WEDRA** (как и репозиторий). Бинарник — `wedra`
> (до v0.26 — `orchestrator`), модуль — `wedra`, ассеты релизов — `wedra_<os>_<arch>`.
> В старых доках/инструкциях — старое имя; команды: `orchestrator …` → `wedra …`.
> M1–M5 закрыты, M6 GUI в работе (срезы 1–3: консоль, браузерный гейт, редактор). Честная версия: **v0.8**.

**Проверено снаружи (M5, v9.1):** 4 внешних автора, 10 плагинов, 8+1 пайплайнов, 0 провалов, ядро 8.5–9/10. Провенанс: волны squash-нулись в первый коммит репо, source в registry.yaml — сам репо wedra (после переезда), поэтому из git это не читается — ограничение видимости, не сокрытие.

> «каждый кусок можно независимо написать, протестировать и заменить» · «контракт честный»

```
wedra/
├── VERSION              # 0.21 (читает бинарник, CWD в приоритете)
├── registry.yaml        # реестр: 23 плагина + 12 пресетов, формат заморожен (commit-пины)
├── cmd/
│   ├── wedra/           # точка входа (CLI + REST API)
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
├── plugins/             # official/ 5, community/ 18 (OSINT: maigret, holehe, crtsh, the_harvester)
├── examples/            # 19 пайплайнов (демо v0.20: when/foreach/parallel; v0.3/v0.4: osint)
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
go build -o wedra ./cmd/wedra
./wedra version   # v0.26
go test ./...            # 167 тестов

# плагины
./wedra plugin validate plugins/csv_loader
./wedra plugin test plugins/csv_loader   # 7 PASS
./wedra plugin test plugins/email_triage # 10 PASS

# пайплайны
./wedra pipeline validate examples/email_check.yaml
./wedra pipeline lint examples/csv_foreach.yaml
./wedra pipeline plan examples/csv_foreach.yaml

# v0.12: foreach по результату шага (было только input.*)
./wedra pipeline run examples/csv_foreach.yaml --yes
# фаза 1: load rows → фаза 2: foreach row → check → review, ok=2

# v0.12: --resume
./wedra runs list
./wedra runs show <run_id>
./wedra pipeline run examples/email_triage_chain.yaml --yes --resume=<run_id>
./wedra runs resume <run_id> examples/email_triage_chain.yaml --yes

# совместимость tool
./tool run examples/csv_foreach.yaml --yes
./tool runs list
```

## v0.16: install-путь («взял и использовал»)

Плагин или пресет — не из локального каталога, а из реестра (`registry.yaml`,
включён в это репо):

```bash
# пресет из реестра + АВТОУСТАНОВКА его плагинов в plugins/
./wedra pipeline install email_check
./wedra pipeline run examples/email_check.yaml --yes

# плагин из реестра (или с пином версии)
./wedra plugin install text_analyzer
./wedra plugin install my_summarizer@v0.16

# свой реестр (оффлайн: каталог с registry.yaml) — без сети
./wedra pipeline install my_preset --registry=./my_registry
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

## Что нового в v0.8 (desktop-exe: двойной клик = окно с GUI)

v0.7 дал автономный `wedra.exe` (GUI встроен, но запуск — из терминала +
сам открыть браузер). v0.8 добавляет **`wedragui-windows-amd64.exe`** —
desktop-приложение:

- **Двойной клик → окно «WEDRA»** (1280×860, по центру) с полным GUI:
  консоль (раны, live-журнал, DAG) + редактор (`/editor/`). Внутри —
  тот же встроенный сервер (v0.7) на случайном порту 127.0.0.1.
- **Техника**: Microsoft WebView2, чистый Go (библиотека `jchv/go-webview2`,
  без CGO) — exe кросс-собирается где угодно (CI: ubuntu). Loader-DLL
  вшита (go-winloader); нужен только рантайм WebView2 — **встроен в
  Windows 10 1803+** (на актуальных Windows есть всегда).
- **Честный фолбэк**: нет рантайма WebView2 → URL в консоль и в лог +
  системный браузер; процесс живёт, пока открыта консоль (Ctrl+C — стоп).
- **Каталоги — рядом с exe** (при двойном клике CWD = папка exe):
  `plugins/` (плагины), `pipelines/` (ваши YAML — консоль их видит),
  `runs/` (журналы). Лог: `%APPDATA%\WEDRA\wedragui.log`.
- **Выход**: закрыть окно. На фолбэк-пути — закрыть консоль.
- **Остальные ОС**: `wedragui` собирается и для linux/darwin — GUI в
  системном браузере (dev-путь); окно — Windows-only (WebView2).
- **Честность про проверку**: windows-бинарник кросс-компилируется
  (CI + здесь) и сверяется на PE-валидность; запустить его в linux-песочнице
  нельзя — runtime-проверка окна (и фолбэка) выполняется вами на Windows.
  Серверная часть (GUI/API/лог/каталоги) прогнана live на linux.
- CI: +2 шага (cross-build wedragui 4 цели + PE-чек; live-прогон в чистом
  каталоге: GUI + редактор + лог + чистый exit).

## 

До v0.7 GUI жил в `web/static/` на диске (относительно CWD): бинарник из
GitHub Release без репо показывал «GUI postponed in v0.12». Теперь:

- **`go:embed`**: весь GUI (консоль + редактор) вшит в бинарник
  (package `web`, `//go:embed static`). `wedra-windows-amd64.exe` из
  Release = **полный продукт**: скопировал файл, `wedra.exe gui`,
  открыл браузер — GUI есть, репо и web/static не нужны, на любой ОС.
- **Dev-режим без потерь**: если `web/static` виден из CWD (запуск из
  checkout) — frontend отдаётся с диска: правка JS/HTML без пересборки.
- **Вся cross-матрица собрана и проверена**: 3 ОС × 2 арх × (wedra + tool)
  = 12 бинарников, `wedra-windows-amd64.exe` — валидный PE32+ (7.9 МБ),
  linux-бинарник прогнан standalone из пустого каталога (GUI + API +
  version 0.7 из ldflags).
- **Версия**: release-воркфлоу штампует `internal/api.Version` через
  `-ldflags` (как с v0.17); из checkout — VERSION-файл, как раньше.
- **Тест**: `TestGUIEmbeddedStatic` — frontend из embedded FS (тесты
  гонятся из internal/api, где нет web/static): `/`, `/editor/`,
  `/editor/app.js`, `/app.js` — 200 + маркеры контента, чужое — 404.
  Тестов 182.

## 

Редактор управляет **сетевой политикой** пайплайна: `network: deny` /
отсутствие (allow). Из «ручного YAML» остался последний ручное поле —
type-объявления в `input`.

- **UI**: блок «network» в панели «Пайплайн» — чекбокс «deny — запретить
  сеть». В свойствах шага — подсказка «сеть плагина: api.openai.com:443
  (declare-now, аудит — журнал)»; если политика deny, а плагин сеть
  заявил — подсказка красным (это и есть ошибка валидации).
- **Кросс-чек (ядро, v0.17)**: `permissions.network` манифеста +
  `network: deny` — **ошибка** в валидаторе и раннере (declare-now:
  честный сетевой плагин не спрятается за «пайплайн не просил»).
  Раннер передаёт шагу `WEDRA_NETWORK=allow|deny`, заявленная сеть
  пишется в журнал (`step_start.network_declared`).
- **Round-trip чистый**: allow = поля в YAML нет (`omitempty`), deny =
  ровно `network: deny`. Значений всего два (раннер: всё, что не deny,
  — allow), поэтому в UI — только чекбокс, без свободного ввода.
- **/api/plugins**: `permissions.network` в нижних ключах
  (`{host, port, any_host, note}`) — на этом строится подсказка в шаге.
- **Пример text_stats** теперь честно объявляет `network: deny` —
  полностью офлайн-пайплайн (текст + человек) с запретом сети.
- Тесты +3 (181 всего): parse text_stats → network в doc; deny round-trip
  + allow-omitempty; конфликт «плагин с сетью + deny» → ошибка, без deny
  → warning с host:port.

## 

Редактор управляет **политикой секретов** пайплайна: `secrets: [ENV_KEY]` —
env-ключи, которые ядро передаст плагинам. Из «ручного YAML» список сократился:
остались только `network` и type-объявления input.

- **UI**: блок «secrets» в панели «Пайплайн» — чипы env-ключей (× убрать,
  поле + Enter добавить). В свойствах шага — живая подсказка: «плагин просит
  ключи X — объявлены ✓ / не объявлены (красным)».
- **Кросс-чек (ядро, v0.17)**: `pipeline.secrets ↔ permissions.secrets`
  манифестов — warnings в бейдже валидации: недообъявленный ключ плагина,
  ключ, который не просит никто, ключ без env. Ошибкой не являются (ядро:
  предупреждение до запуска) — решение за пользователем, честность за ядром.
- **Round-trip чистый**: без секретов в YAML поле не прописывается
  (`omitempty`); с секретами — ровно как в исходнике. Пустые/пробельные
  имена ключей из UI отбрасываются.
- **/api/plugins** теперь отдаёт `permissions` манифеста (на этом строится
  подсказка в шаге) — аддитивно, старые поля не тронуты.
- **Примеры llm_same_provider / llm_text_chain** теперь честно объявляют
  свои ключи (`GEMINI_API_KEY`, `LLM_OAI_API_KEY`) — валидация без
  warnings о секретах.
- Тесты +3 (parse llm_same_provider: secrets в doc; round-trip на фикстуре
  с permissions.secrets; недообъявленный ключ → warning, ok=true).

## 

Вторая волна OSINT-плагинов — теперь и доменные цепочки штатные.

- **`crtsh`** (plugins/community/crtsh): **встроенный, без зависимостей** —
  чистый stdlib-HTTP к crt.sh (Certificate Transparency): wildcard-запрос
  `%domain`, выход агрегатами `{total, names[], issuers[], expired}`
  (сырьё — тысячи записей — в выход не тащим). Сеть declare-now на
  конкретный хост: `{host: crt.sh, port: 443}`.
- **`the_harvester`** (plugins/community/the_harvester): обёртка над CLI
  theHarvester 4.9.2 (laramies) — emails/имена/субдомены/ASNs по домену из
  поисковиков, CT и соцсетей (`-b crtsh,hackertarget,google,...`).
  Выход — нормализованные массивы: emails/hosts/people/vhosts/asns/
  interesting_urls (отсутствует в репорте → []).
  **Установка из git**: `pip install git+https://github.com/laramies/theHarvester.git@4.9.2`
  — на PyPI `theharvester` 0.0.1 чужой squatted-пакет (мастер-ветка
  требует Python ≥3.14, 4.9.2 — совместима с 3.13).
- **Пример** `examples/osint_domain_audit.yaml`: domain → **ПАРАЛЛЕЛЬНО**
  (parallel_group) crtsh + theHarvester → human_gate (total/expired/
  emails/hosts). on_error: retry — механика v0.29.
- **Live-проверено в песочнице** (реальные запуски): crt.sh example.com →
  77 сертификатов, 61 просрочен, имена CN+SAN с корректным разрезом по
  `\n` (баг пойман на живых данных); theHarvester → 500 хостов из CT;
  **crt.sh отдал 502 прямо в ране → retry (exponential) вытянул второй
  attempt — механика v0.29 отработала на живом сценарии впервые**.
- **sn0int — отложен честно**: в песочнице не собирается (устаревшая
  зависимость hlua-badtouch несовместима с актуальным lua-ml, prebuilt-
  бинарников в releases нет) — обещать непроверенный вывод не будем.
- Контракт-тесты +14 (crtsh: 5 — mock HTTP-ответ; the_harvester: 9 —
  mock CLI) — 124 кейса по 23 плагинам, CI без сети.

## 

Первые «внешние» плагины-обёртки: цепочки вида «нашёл ник → проверил email
→ человек решил» становятся штатным сценарием.

- **`maigret`** (plugins/community/maigret): обёртка над CLI maigret
  (soxrave) — поиск username по базе 4000+ сайтов. Вход: `username`,
  `sites[]` (фильтр, без него — вся база), `timeout`, `wall_timeout`.
  Выход: `found[{site,url,username}]` (только Claimed), `checked`, `claimed`.
  Пустой репорт (ник нигде) — нормальный результат, не ошибка.
- **`holehe`** (plugins/community/holehe): обёртка над CLI holehe (megadose)
  — проверка email/username по 120+ сайтам на утечки. CSV-репорт (JSON у
  holehe нет) → `leaks[{site,domain,method}]`, `checked`, `rate_limited`
  (не проверилось из-за лимитов), `phone_numbers`.
- **Оба честно**: stdlib-only обёртки (subprocess), сам пакет ставится
  отдельно (`pip install maigret` / `pip install holehe`); без него — доменная
  ошибка `*_not_installed` (а не молчаливое «нашёл 0»). Сеть — declare-now:
  any_host 80/443, видно в валидации и аудите журнала. Команду можно
  переопределить env (MAIGRET_BIN / HOLEHE_BIN) — так контракт-тесты гоняют
  mock-CLI без сети.
- **Пример** `examples/osint_username_audit.yaml`: username → maigret →
  email → holehe → human_gate (цифры в форму, решение — человека).
  on_error: retry с exponential backoff — сетевые таймауты повторяются
  (механика v0.29).
- **Live-проверено в песочнице** (реальные запуски): maigret нашёл
  github/gist/wikipedia по публичному нику; holehe прошёл 122 сайта
  (datacenter-IP: 120 rate-limited — честно помечено в выводе).
- **v0.3a: maigret/holehe в реестре** — SHA-пин нового контента неизвестен до
  коммита (дисциплина supply-chain, v0.28a/v0.29), поэтому реестр-записи
  отпустились отдельным буквенным релизом: maigret/holehe — SHA v0.3
  (`61b5c30`), старые 31 запись — версия v0.3a, контент и пины без изменений.
- Контракт-тесты +16 (mock-CLI: парсинг репортов, доменные/платформенные
  ошибки, wall timeout, отсутствующий бинарник по пути и по имени из PATH).

## 

Третий срез «мускулов v1.0»: редактор управляет **политикой повторов**.
llm_* примеры (on_error: retry + retry-блок) теперь редакторские.

- **on_error**: появился вариант `retry` (было stop/skip). Выбор retry без
  блока автоматически заполняет дефолты (3 / 1s / fixed).
- **retry-блок**: `attempts` (≥1), `delay` (5s, 500ms, 1m…), `backoff`
  (fixed | exponential). Семантика ядра: повтор таймаутов и доменных
  ошибок с retryable: true; исчерпанный retry = stop.
- **Честность**: zero-delay в исходнике не прописывается как `0s` (round-trip
  чистый); `on_error: retry` + `attempts < 1` → ошибка валидации в бейдже,
  сохранение заблокировано.
- **Осталось в YAML**: `secrets`, `network`, type-объявления input — это
  сознательный минимум (всё «управляющее» уже в редакторе).
- Тесты +3 (parse llm_same_provider: retry в doc; round-trip на фикстурном
  плагине; attempts=0 → ok:false).

## 

Второй срез «мускулов v1.0»: редактор управляет **всем управляющим потоком**,
кроме retry. Пайплайны foreach_step_demo / parallel_demo / csv_foreach*
теперь открываются, редактируются и сохраняются из редактора.

- **Шаг** (панель свойств): `foreach` — массив (список источников, как в bind)
  + `foreach_item` (переменная элемента, по умолчанию item); `parallel_group`
  — смежные шаги с одним именем исполняются параллельно (барьер до след.);
  `after_foreach` — чекбокс: шаг один раз после всего pipeline-foreach
  (агрегаты steps.X_all). human_gate: foreach/parallel не предлагаться —
  ядро это запрещает (гейт сериализует терминал).
- **Пайплайн** (секция «Батч»): `foreach` (input.* или steps.X.out),
  `foreach_item`, `item_type`, `item_format`.
- **Честность**: конфликты ядра (foreach + after_foreach, foreach +
  parallel_group, foreach у гейта, путь из ещё не выполненного шага)
  показывается валидацией — бейдж в редакторе, сохранение заблокировано.
- **Осталось в YAML**: `retry`, `secrets`, `network`, type-объявления input.
- Тесты +4 (parse трёх реальных примеров; pipeline-foreach round-trip на
  гейтах; step-foreach + parallel на фикстурном плагине; конфликты →
  ok:false).


Первый срез «мускулов v1.0»: редактор управляет **условием шага** (`when`) —
раньше такой пайплайн открывался с баннером и без сохранения.

- **Панель свойств** (шаг): селект «when» — 10 операторов ядра (truthy,
  exists, missing, eq, neq, gt, gte, lt, lte, contains), `when.path` — список
  источников (input.* + steps.X.out, как в bind; чужой путь — «вручную»),
  `when.value` — значение (для числовых операторов).
- **Честность**: value из UI приходит строкой — для gt/gte/lt/lte редактор
  коэрсирует в число (иначе ядро на рантайме не сравнит), пустое value не
  пишется в YAML. Сгенерированный YAML по-прежнему обязан читаться ядром и
  проходить валидацию (неизвестный op отловится самопроверкой → ошибки).
- **На узле** — чип `⚖ when:<op>` в футере.
- **Осталось в YAML вручную** (на v0.27): `foreach`, `parallel_group`,
  `after_foreach` (с v0.28 — в редакторе), `retry`, `secrets`, `network`,
  type-объявления input.
- when_demo.yaml теперь **полностью редакторский** (без type-объявлений) —
  CI проверяет round-trip: parse → when в doc → serialize ok.


Продукт переименован в **WEDRA** — совпало с репозиторием, баннером и
`WEDRA_NETWORK` (так переменная сети называлась ещё с v0.17 — имя выдержало
время, остальное догнало).

- **Бинарник**: `orchestrator` → **`wedra`** (`cmd/wedra`). После
  переустановки команда — `wedra`; алиасы/скрипты — одна строка.
- **Модуль**: go.mod `module wedra`, импорт-пути `wedra/internal/...`.
- **Релиз**: ассеты `wedra_<os>_<arch>` (12, как раньше) + legacy `tool`.
- **GUI**: шапки консоли и редактора — WEDRA; комментарий в генерируемых
  YAML — «создан в редакторе WEDRA».
- **Доки**: README, docs/, CONTRIBUTING, PROTOCOL, registry — новое имя.
  CHANGELOG и archive — история, не переписываем (там старое имя — честно).
  Текст демо-примеров не тронут (это входные данные, не бренд).


Мёртвый прототип M5 (/editor/) переписан с нуля и поднят в первый класс
(ссылка в консоли снова есть — теперь на работающий инструмент).

- **Холст**: палитра плагинов (из реестра, живым списком) + встроенный
  `core/human_gate`; drag-нdrop из палитры, узлы двигаются по сетке 20px
  (позиции сохраняются в YAML как `pos: [x y]` — ядро поле игнорирует,
  редактор читает обратно).
- **Связи**: bind выбирается из списка источников (`input.*`,
  `steps.<шаг>.<выход>`; у гейта — из его формы, по правилам материализации
  ядра), рёбра рисуются как кривые и перерисовываются при каждом изменении.
- **Шаг**: id (переименование переносит чужие ссылки), on_error (stop/skip),
  timeout, bind по каждому входу плагина; гейт: form (построчно, `e:` =
  editable), actions, on_reject.
- **Undo/redo** (Ctrl+Z / Ctrl+Y, 100 шагов), Delete — удалить шаг.
- **YAML пишет ядро**: парсинг и сериализация — Go-эндпоинты
  `POST /api/parse/pipeline`, `POST /api/serialize/pipeline`. Браузер не
  хранит и не генерирует YAML; сгенерированный файл обязан читаться лоадером
  и проходить валидацию (эндпоинт так проверяет сам себя).
- **Честный скоуп**: `when` (v0.27), `foreach`/`parallel_group`/
  `after_foreach` (v0.28) и `retry` (v0.29) под управлением; `secrets`,
  `network` и type-объявления в input редактор не управляет — такие пайплайны
  открываются с баннером, а сохранение из редактора запрещено (409) — данные
  не теряются, правь в YAML во вкладке «Пайплайны».
- Сохранение: `PUT /api/pipelines/<file>` (существующий механизм, валидация
  до записи), новый файл создаётся.

Тесты: +13 (v0.27: when; v0.28: foreach/parallel; v0.29: retry parse
llm_same_provider, round-trip, attempts=0 → ok:false).
CI: live-шаг «v0.25 editor» (node --check + parse examples; v0.27: when;
v0.28: foreach/parallel; v0.29: retry-структура + attempts=0 → ok:false).


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


`wedra gui` больше не «отложен, косметика» — это рабочая консоль
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

Кто угодно: `wedra plugin install word_freq` — и плагин у вас в `plugins/`.
Свой плагин — `docs/plugin-dev.md` (15 минут) + `OUTREACH_ROUND2.md` (меню идей).

## Что нового в v0.17 (trust)

- **`wedra registry validate [--registry=<src>] [--local-source=<dir>]`** —
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
- CLI: `wedra pipeline run --resume=<id>`, `wedra runs resume <id> <yaml>`

**3. runs CLI**
- `wedra runs list [var/runs]` — список прогонов с pipeline name и events count
- `wedra runs show <id>` — полный журнал + context snapshot
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
- [x] v0.22: **GUI-консоль** — live-терминал, таймлайн, DAG, запуск --yes из браузера (`wedra gui`)
- [x] v0.23: **контракт рантайма** — типы/форматы на входе и выходе, гейт без молчаливого accept, атомарный журнал, group kill, лимиты вывода
- [x] v0.24: **человек в гейте из браузера** — гейт-карточка в консоли, решение в живой ран (API + `gate_wait`/`gate_retry`), ссылка на мёртвый /editor/ убрана
- [x] v0.25: **редактор пайплайнов** — палитра/сетка/связи/undo, YAML пишет ядро (parse/serialize API), honest-scope: when/foreach → YAML
- [x] v0.25a: баннер WEDRA в README (первый буквенный релиз по схеме)
- [x] v0.26: **имя WEDRA** — бинарник `wedra`, модуль `wedra`, ассеты `wedra_*`, доки переименованы
- [x] v0.26a: редактор сохраняет `format_version` исходного файла (было хардкод `0.1`); новое doc → `0.2`
- [x] v0.27: **when в редакторе** — 10 операторов, path из источников, value с коэрсией; when_demo полностью редакторский
- [x] v0.28: **foreach/parallel_group/after_foreach в редакторе** (шаг + pipeline-батч); из ручного списка осталось retry/secrets/network
- [x] v0.28a: security-фиксы аудита — path traversal (400), CSRF (403), commit-пин в реестре (fail-closed), README-артефакты починены
- [x] v0.29: **retry в редакторе** (on_error: retry + attempts/delay/backoff); из ручного списка осталось secrets/network/type-input
- [x] v0.3: **OSINT-плагины** — maigret (ник по 4000+ сайтам) + holehe (email по 120+) + пример osint_username_audit; в реестре — с v0.3a (SHA-пины)
- [x] v0.3a: **maigret/holehe в реестре** (SHA-пины на v0.3) — install-путь для OSINT-цепочки
- [x] v0.4: **OSINT-аудит домена** — crtsh (встроенный, stdlib-HTTP) + the_harvester (CLI 4.9.2 из git) + пример с parallel_group; live: retry на живом 502 crt.sh
- [x] v0.4a: **crtsh/the_harvester в реестре** (SHA-пины на v0.4) — install-путь для доменного аудита
- [x] v0.5: **v1.0-трек, срез 1: secrets в редакторе** (чипы env-ключей + подсказка по плагину + round-trip); ручной список: network, type-input
- [x] v0.6: **v1.0-трек, срез 2: network в редакторе** (политика allow/deny + подсказка по плагину); ручной список: type-input (последнее)
- [x] v0.7: **v1.0-трек, release-срез: GUI вшит в бинарник (go:embed)** — exe из Release = полный продукт без репо (12 бинарников, 3 ОС × 2 арх)
- [x] v0.8: **v1.0-трек: desktop-exe** — `wedragui-windows-amd64.exe`: двойной клик = окно WEDRA (WebView2, чистый Go, без CGO); фолбэк в браузер; каталоги рядом с exe
- [ ] v1.0: GUI full (редактор + when/foreach в UI, import) + маркетплейс v1 (гейт из браузера — v0.24, редактор — v0.25)

## Тесты

`go test ./...` — 182 теста PASS, `csv_foreach` зелёный (ok=2), resume — все элементы уже пройдены, install- и trust-сценарии покрыты e2e. Контракт-тесты плагинов (`tool plugin test`) — 124 кейса по всем 23 плагинам (CI: каждый релиз).

## Версионирование

- v0.9 (ex v9), v0.9.1 (ex v9.1), v0.10 (ex v10), v0.11 (GUI scaffold), v0.12 (CLI focus), v0.13 (честный перенос), v0.14 (JsonStore — тогда ещё назывался SQLiteStore, в v0.15 переименован честно), v0.15 (честный релиз), v0.16 (install-путь), v0.17 (trust), v0.18 (волна 2: community-плагины), v0.18.1 (долги), v0.19 (волна 2, батч 2), v0.20 (управляющий поток), v0.21 (хирургия структуры), v0.22 (GUI-консоль), v0.23 (контракт рантайма), v0.24 (браузерный гейт), v0.25 (редактор пайплайнов), v0.25a (баннер, первый буквенный), v0.26 (имя WEDRA), v0.26a (format_version в редакторе), v0.27 (when в редакторе), v0.28 (foreach/parallel в редакторе), v0.28a (security-фиксы по аудиту), v0.29 (retry в редакторе), v0.3 (OSINT-плагины maigret + holehe), v0.3a (maigret/holehe в реестре, SHA-пины), v0.4 (OSINT-аудит домена: crtsh + the_harvester), v0.4a (crtsh/the_harvester в реестре, SHA-пины), v0.5 (secrets в редакторе), v0.6 (network в редакторе), v0.7 (GUI в бинарнике — exe из Release = полный продукт), v0.8 (desktop-exe: двойной клик = окно с GUI)
- Дальше: when/foreach/parallel в UI редактора → v1.0 — GUI full + маркетплейс (имя WEDRA — решено, v0.26)
- **Схема букв (с v0.23, договорённости):** цифра = функциональный срез;
  буква = фикс-релиз внутри среза без новых фич (v0.24a, v0.24b…).
  Отпущенный tag больше не сдвигается. После `z` — срез был объявлен рано,
  поднимаем цифру. (В v0.23 я два раза force-moved тег под race- и
  кросс-сборочные фиксы — с v0.24 так не будет: фикс = v0.23a-стиль.)
