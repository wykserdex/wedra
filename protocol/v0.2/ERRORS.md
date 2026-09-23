# ERRORS v0.2 — коды ошибок для LLM-агентов

Коды — стабильный публичный контракт. Тексты `message` можно менять, коды нельзя.
`ok:false` в `validate` — это нормальный результат, не исключение: агент читает
`issues[0].code`, берет `hint`/`fix.candidates`, исправляет YAML и повторяет.

## Формат Issue

```json
{
  "code": "E_TYPE_MISMATCH",
  "severity": "error",
  "step": "triage",
  "port": "email",
  "path": "steps.triage.email",
  "message": "шаг triage, порт email: тип string несовместим с выходом ...",
  "hint": "нужен тип ...",
  "fix": {"op": "bind", "target": "steps.triage.email", "candidates": ["steps.syntax.email"]}
}
```

- `severity`: `error` блокирует запуск, `warning` — нет.
- `path`: `pipeline.*`, `pipeline.steps.<id>.*`, `steps.*`, `input.*`, `input.<arr>[<i>]`.
- `fix.op`: `bind` — перепривязать порт; `set` — выставить значение;
  `declare` — объявить недостающее (secrets).
- `fix.candidates` отсортирован, пуст — если подсказать нечего.

## Ошибки валидации (pre-run)

| Код | Когда | Подсказка |
|---|---|---|
| E_FORMAT_VERSION | `format_version` не 0.1/0.2 | `fix.candidates: ["0.1","0.2"]` |
| E_CYCLE | цикл в DAG | разорвите зависимость |
| E_FOREACH_PATH | `pipeline.foreach` не с `input.`/`steps.` | `fix.candidates: input.*` |
| E_FOREACH_SHAPE | `steps.*` не вида `steps.<id>.<field>` | формат в hint |
| E_FOREACH_NOT_FOUND | массив/шаг для foreach не найден | `fix.candidates: input.*` |
| E_STEP_ID_EMPTY | пустой `id` | дайте id |
| E_STEP_ID_DUP | дубль `id` | уникальные id |
| E_WHEN_OP | неизвестный `when.op` | `fix.candidates: [truthy,exists,...]` |
| E_WHEN_PATH | `when.path` не с `input.`/`steps.` | — |
| E_WHEN_FORWARD_REF | `when` читает шаг ниже по списку | только шаги выше |
| E_STEP_FOREACH_PATH | `steps.<id>.foreach` плохой путь | — |
| E_STEP_FOREACH_SHAPE | `steps.<id>.foreach` не `steps.<id>.<field>` | — |
| E_STEP_FOREACH_NOT_FOUND | массив для step-foreach не найден | `fix.candidates: input.*` |
| E_FOREACH_COMBO | `foreach` + `after_foreach`/`parallel_group` | уберите одно |
| E_FOREACH_ITEM_NAME | `foreach_item` не простое имя | буквы/цифры/`_` |
| E_GATE_FOREACH | `human_gate` с `foreach` | гейт — один набор полей |
| E_GATE_PARALLEL | `human_gate` в `parallel_group` | вынесите гейт |
| E_GATE_BIND | `human_gate` с `bind` | данные — через `form` |
| E_ON_ERROR | `on_error` не stop/skip/retry | `fix.candidates` |
| E_RETRY_ATTEMPTS | `retry.attempts < 1` | `>=1` |
| E_ON_REJECT | `on_reject` не stop/continue | `fix.candidates` |
| E_PLUGIN_LOAD | плагин не загрузился | проверьте путь и `plugin.yaml` |
| E_BIND_UNKNOWN_PORT | `bind` на несуществующий порт | `fix.candidates: порты плагина` |
| E_NETWORK_DENIED | плагин заявил сеть, а `network: deny` | уберите сеть или deny |
| E_PORT_UNBOUND | обязательный порт без привязки | `fix.candidates: input.* + steps.*` |
| E_PORT_SOURCE | источник не резолвится (`в input нет поля`, `шаг не найден выше`, `плагин не объявляет выход`) | `fix.candidates: input.* + совместимые steps.*` |
| E_TYPE_MISMATCH | тип источника ≠ типу порта | `fix.candidates: совместимые steps.*` |
| E_FORMAT_INPUT | литерал `input` не соответствует `format` | исправьте значение |
| E_FORMAT_MISMATCH | формат источника не покрывает формат порта | `fix.candidates: совместимые steps.*` |
| E_OPTIONAL_REQUIRED | читает из `skip`-able шага, но порт не `optional` | сделайте `optional` |
| E_PARALLEL_SPLIT | шаги `parallel_group` не рядом | поставьте рядом |
| E_FILE_REF_NOT_FOUND | `file_ref` файл не найден | положите рядом с плагином |
| E_APPROVAL_VALUE | `approval` не human/any | `fix.candidates: [human,any]` |
| E_GATES_VALUE | `pipeline.gates` не human_only/any | `fix.candidates` |
| E_MANIFEST_* | битый `plugin.yaml` (`VERSION`, `PLATFORM_API`, `RUNTIME`, `ENTRY`, `INPUT_TYPE`, `FORMAT`, `OUTPUT_EMPTY`) | чините манифест |

## Предупреждения (pre-run, не блокируют)

| Код | Когда |
|---|---|
| W_FORMAT_VERSION_MISSING | нет `format_version` |
| W_SECRETS_MISSING_ENV | `secrets:` env не задан |
| W_FOREACH_ITEM_TYPE | `input.<arr>[i]` не тот `item_type` |
| W_FOREACH_ITEM_FORMAT | `input.<arr>[i]` не соответствует `item_format` (плохой элемент уронит только себя) |
| W_GATE_FORM_COLLISION | коллизия basename в `form` |
| W_GATE_FORM_MISSING | `form` поле может отсутствовать |
| W_GATE_FORM_SKIP | `form` читает из `skip`-able шага |
| W_NETWORK_DECLARED | плагин заявил сеть (аудит — журнал) |
| W_PORT_OPTIONAL_UNBOUND | `optional` порт без привязки |
| W_PORT_OPTIONAL_SOURCE | `optional` порт с битым источником |
| W_SECRETS_UNUSED | `pipeline.secrets` никто не просит |
| W_SECRETS_UNDECLARED | плагину нужен ключ — объявите в `pipeline.secrets` |
| W_PARALLEL_SINGLE | `parallel_group` из одного шага |
| W_FILE_REF_ROOT | `file_ref` найден от корня, но не от плагина |

## Ошибки рантайма (журнал: `step_failed` / `run_failed` → `code`)

| Код | Когда | Судьба элемента/рана |
|---|---|---|
| contract_input | вход шага не собрался (нет пути, тип/формат) | элемент: `skip→ok`, иначе `aborted`; ран живёт |
| contract_output | плагин не вернул обязательное поле или соврал типом/форматом | так же, как `contract_input` |
| platform:\<code\> | платформенная ошибка (`timeout`, `crash`, `spawn_failed`, `protocol_violation`, `platform:<code плагина>`) | ран остановлен |
| when_error | `when` не вычислился | ран остановлен |
| foreach_path | `foreach` путь не найден / не массив | ран остановлен |
| secrets_missing | нет env из `pipeline.secrets` | ран не стартует |
| network_denied | `network: deny` нарушен в рантайме | ран не стартует |
| run_error | прочее (резолв, resume, параллельная группа) | ран остановлен |
| cancelled | отмена (Фаза 5) | `run_cancelled`, затем snapshot |

`step_skipped` с `reason:on_error` несёт исходный `code` плагина (`bad_syntax`, ...).
`gate_decision` несёт `source`: `terminal` / `gui` / `auto_yes`.

## Human-only гейты (модель угроз — честно)

- `approval: human` на шаге и `pipeline.gates: human_only` запрещают авто-аппрув
  (`--yes` не одобряет, ждёт человека). Для раннов из MCP авто-аппрув выключен всегда.
- Мутирующие HTTP (`POST /api/run`, `POST /api/runs/<id>/gate`, `POST .../cancel`)
  требуют cookie сессии человека (`wedra gui` печатает ссылку `?k=<secret>` в терминал
  человека; секрет — только в памяти и cookie, в журнал пишется только хэш `session`).
  Без cookie — 401. Агент через MCP не имеет инструмента одобрения; `get_run`
  в статусе `waiting_human` говорит «попросите пользователя одобрить в окне wedra».
- Это защита от того, что агент **случайно или по инструкции** одобрит сам себя
  через доступные ему инструменты. От злонамеренного процесса того же пользователя ОС
  (чтение памяти, правка файлов) она не защищает — такая формулировка честнее,
  чем «security boundary».
