# orchestrator — скелет MVP

Локальный оркестратор цепочек с человеком в петле. Это воплощение этапов **M1–M3** из ТЗ v2 (`../tz_platform_v2.md`): протокол заморожен, два референс-плагина написаны вручную, ядро гоняет цепочку end-to-end из CLI.

**Проверено снаружи (грязный публичный тест, M5):** 3 внешних автора, 5 плагинов из 4 незапланированных ниш, 3/3 написали свой плагин с первого захода, провалов воронки — 0; ядро оценено извне **~9/10 для MVP**. Их слова:

> «каждый кусок можно независимо написать, протестировать и заменить» ·
> «ошибки говорят буквально, какой порт и почему несовместим» ·
> «не пришлось писать SDK или наследоваться от класса»

```
orchestrator/
├── PROTOCOL.md          # контракт ядро↔плагин (v0.1, единственный «вечный» документ)
├── cmd/tool/main.go     # CLI: run / validate / plugin-validate
├── internal/core/       # ядро: раннер, экзекутор, валидация, журнал, human_gate
├── plugins/             # референс-плагины (любой язык — лишь бы протокол)
│   ├── syntax_mx_checker/   # OSINT: синтаксис + MX (пак A)
│   ├── disposable_checker/  # OSINT: disposable-домены, офлайн (пак A)
│   ├── llm_gemini/          # Gemini generateContent (пак B)
│   ├── llm_openai/          # OpenAI-совместимый: OpenAI/Grok/DeepSeek/Ollama (пак B)
│   ├── llm_anthropic/       # Anthropic Messages API (пак B)
│   ├── text_analyzer/       # community-плагин №1: метрики текста (автор — тестер M5)
│   └── dir_lister/          # community-плагин №2: снапшот директории; манифест БЕЗ from (v0.2)
├── pipelines/
│   ├── email_check.yaml     # пак A: foreach 3 email + human_gate
│   ├── single_check.yaml    # одиночный прогон — CI-семантика + пример bind (v0.2)
│   ├── llm_text_chain.yaml  # пак B: Gemini → человек → OpenAI/Grok
│   ├── llm_same_provider.yaml # пак B': один провайдер ДВАЖДЫ через bind (v0.2)
│   ├── text_stats.yaml      # пак C: метрики текста → обзор человеком
│   └── dir_snapshots.yaml   # аудит "до/после": один dir_lister дважды через bind (v0.2)
└── runs/                # журналы прогонов (jsonl + снапшоты контекста)
```

## Быстрый старт

> Хочешь писать СВОЙ плагин? → `TUTORIAL_PLUGINS.md` (15 минут, выверен на граблях трёх внешних авторов).

Требования: Go ≥ 1.21, Python ≥ 3.9 (опционально `pip install dnspython` для MX-проверки).

```bash
go build -o tool ./cmd/tool
go test ./internal/core/                            # 93 теста: ядро + плагины + SDK покрыты

./tool plugin validate plugins/syntax_mx_checker   # контракт плагина (манифест)
./tool plugin test plugins/syntax_mx_checker       # контракт-тесты из plugin.test.yaml
./tool validate pipelines/email_check.yaml         # статическая проверка цепочки
./tool run pipelines/email_check.yaml              # интерактивно (человек в петле)
./tool run pipelines/email_check.yaml --yes        # auto-accept (для CI/демо)

# пак B (LLM-конвейер). Без ключей — mock-режим:
LLM_MOCK=1 ./tool run pipelines/llm_text_chain.yaml
# пак B' (v0.2, bind): Gemini и черновиком, и доработкой:
LLM_MOCK=1 ./tool run pipelines/llm_same_provider.yaml
# С ключами (Grok вместо OpenAI — через base_url):
GEMINI_API_KEY=... LLM_OAI_API_KEY=... \
LLM_OAI_BASE_URL=https://api.x.ai/v1 LLM_OAI_MODEL=grok-3-mini \
  ./tool run pipelines/llm_text_chain.yaml
```

Интерактивный режим: на `human_gate` показываются поля формы, `*` — редактируемые
(новое значение вводится как JSON, проверяется по типу), затем действие `a`/`r`.

## SDK: контракт-тесты плагина (plugin.test.yaml)

Каждый плагин несёт свой тест-набор рядом с кодом; прогон идёт через **тот же** subprocess-протокол, что и в ране:

```yaml
tests:
  - name: mailinator → disposable=true
    input: { email: "user@mailinator.com" }        # JSON на stdin
    expect:
      status: ok
      output:
        disposable: true                           # точное (глубокое) равенство
        domain: { contains: "mailinator" }         # матчер

  - name: без ключа → понятная ошибка, не traceback
    env: { LLM_OAI_API_KEY: "" }                   # env на время теста (секреты, mock)
    input: { prompt: "x" }
    expect: { status: error, exit_code: 1, error: { code: no_api_key, retryable: false } }

  - name: битый JSON → платформенная ошибка
    input_raw: "{oops"                             # сырой stdin
    expect: { exit_code: 2 }
```

Матчеры полей output: литерал (глубокое равенство, YAML int ≡ JSON float64), `{ present: true }`, `{ contains: "..." }` — по строке И по массиву (элемент, глубокое сравнение), `{ type: "boolean|number|string|array|object" }`, `{ equals: ... }`. Прогон включает enforce контракта: незадекларированный выход — warning; пропавший обязательный или **вернувшийся с другим типом** — провал: `контракт: поле "value" объявлено как string, вернулось number`. Любой провал → exit 1 (годится как CI-гейт для будущего маркетплейса). А `plugin validate` дополнительно отклоняет манифесты уровня «`format: json` на `type: array`» — format применим только к строкам, сообщение говорит об этом на месте, а не головоломкой в пайплайне.

## Что уже работает

- subprocess-протокол stdin/stdout JSON + exit codes (0/1/≥2 + таймаут)
- Shared Context с неймспейсами `steps.<step_id>`; плагин видит только свой вход
- enforce контракта: незадекларированные выходы отбрасываются (warning в журнал), пропавшие обязательные — стоп рана
- статическая валидация до запуска: пути, типы, форматы, skip-безопасность (`tool validate`)
- политики ошибок `stop | skip | retry` (attempts/delay/backoff); доменные vs платформенные ошибки
- `foreach` — батч-режим со scope-семантикой: stop останавливает элемент, не ран
- `core/human_gate` в CLI: форма по полям, правки с проверкой типа, **материализация полей на accept** (downstream читает `steps.<gate>.*`, источник не затирается), reject → on_reject
- пак B: три LLM-адаптера (Gemini, OpenAI-compatible, Anthropic) — stdlib-only, секреты через env, 429/5xx → retryable, mock-режим `LLM_MOCK=1` для демо без ключей
- `tool plugin test` — контракт-тесты автора плагина: plugin.test.yaml + реальный протокол + enforce + матчеры (см. ниже)
- `tool plugin create <path> [--author N] [--description "..."] [--example string|array]` — генератор скелета: манифест с комментариями-учебником (+шпаргалка типов валидатора), main.py как урок протокола (в т.ч. runtime-guard для array-входов), стартовые зелёные тесты, защита от плохого id и перезаписи чужой папки; флаги в любом порядке
- **контракт v0.2: `bind`** — разводка входов в YAML-шаге поверх дефолтов манифеста; один плагин может встречаться в цепочке дважды (demo: `llm_same_provider.yaml`, `dir_snapshots.yaml`); статика ловит опечатки портов, отсутствие привязки, type/format mismatch и через bind тоже
- **⚠ про `file_ref` до запуска** — cwd subprocess плагина = его собственная директория; если относительный путь из pipeline не резолвится от неё (но есть от корня проекта) — ядро предупреждает до старта плагина, в консоль и journal
- журнал прогона `runs/<id>/journal.jsonl` + снапшот `context.json` после каждого элемента

## Сознательные упрощения (честный список долгов)

| Долг | Куда мапится в ТЗ |
|---|---|
| Нет `--resume` прерванного рана (журнал уже пишется с этим расчётом) | M2→M4, §4.8 |
| Секреты — только env-переменные, keyring отложен | §4.9 |
| Правки гейта валидируются по типу поля в form, а не по контракту следующего шага | §4.7 |
| `permissions` декларативны (уровень L0), исполнения изоляции нет | §4.5 |
| Правки YAML↔GUI, реестр плагинов, wizard — за пределами скелета | M6, M7 |
| Разводка входов зашита в манифест (`from:`); нужен `bind:` в YAML-шаге для переиспользования плагина | контракт v0.2 (найдено при написании single_check) |

## Тесты

`go test ./internal/core/` — покрытие: контекст/неймспейсы, YAML-парсинг, статическая валидация (несовместимые типы и форматы, skip-безопасность, forward-refs, литеральные format-проверки, коллизии basename в gate, optional без привязки), экзекутор (все классы exit-кодов, таймаут, нарушение протокола, platform:code на exit>=2), enforce контракта, раннер (foreach-scope, retry до успеха/исчерпания, платформенный стоп рана, human_gate auto/reject/правки с квалифицированными ключами при коллизии). Фикстуры — 11 плагинов в `internal/core/testdata/plugins/`, в т.ч. крашащийся и нарушающий протокол.

## Следующие шаги (по ТЗ)

1. **M3 зачтён частично**: три LLM-адаптера + генератор написаны автором ядра — нужен внешний автор по PROTOCOL.md с замером времени (настоящий тест контракта).
2. ~~Покрыть `internal/core` тестами~~ — сделано (64 теста).
3. ~~`tool plugin test`~~ — сделано.
4. ~~`tool plugin create`~~ — сделано; **SDK-триада замкнута**: create → код → test → validate → готово.
5. `--resume <run_id>` из снапшотов журнала.
6. ~~Контракт v0.2: `bind:`~~ — сделано и **подтверждено внешним автором** (пакет №5: аудит «до/после» одним плагином дважды, манифест без `from`).
7. **M5, грязный публичный тест:** три внешних автора по трём профилям, провалов воронки 0, 5 внешних плагинов из 4 ниш (`M5_FEEDBACK.md`); ядро оценено извне **~9/10 для MVP** по итогу «ненулевой нагрузки» (fan-in OSINT-коррелятор); бэклог (`--resume`) заморожен до запроса из фидбека. 93 теста зелёных.
8. **M6 (GUI) — отложен решением автора (28.08):** фокус — техническая часть; дистрибуция = dev-чаты + репа (`POSTS.md`); возврат к GUI — когда ≥2 холодных скажут «без UI не пойду» или закроется 5/5 цели M5.
