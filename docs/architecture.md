# Архитектура — WEDRA

Короткий проводник для нового читателя. Контракт — `protocol/v0.2/PROTOCOL.md`,
здесь — как устроено ядро и почему.

## Принципы

1. **Плагин = процесс.** stdin JSON → stdout JSON, exit code — класс результата.
   Ядро не знает, что внутри плагина (python/го/любой скрипт). Контракт честный:
   «каждый кусок можно независимо написать, протестировать и заменить».
2. **Локально и детерминированно.** Журнал каждого рана — на диске
   (`var/runs/<id>/`), `--resume` собирает прерванный батч. Ничего не живёт
   только в памяти.
3. **Человек в петле — не гостевая фича.** `core/human_gate` — встроенная нода
   ядра (не плагин): форма из контекста, правки материализуются под
   `steps.<gate>.*`, в `--yes` авто-принимается.
4. **Доверие проверяется до любого эффекта.** `validate` — статика (типы,
   форматы, пути, циклы, сети при `network: deny`, secrets), `registry validate`
   — конформность каждой записи реестра (CI-гейт).

## Дерево

```
cmd/wedra        # точка входа (CLI + REST API)
cmd/tool                # compat-шим (M5): run/validate/plugin/runs
internal/
  pipeline/             # модель, парсер, валидатор, планер (DAG), when-условия
  execution/            # раннер: фазы foreach, step-foreach, parallel_group
  plugin/               # процесс, transport, enforce, manifest, registry-разрешение
  registry/             # реестр v0.1, install, pin-контракт (RefToDir)
  gate/                 # human_gate: service + terminal + typing
  journal/              # writer/reader + RunStore (filesystem и JSON-store)
  context/              # shared context, dot-пути
  cli/                  # команды pipeline|plugin|runs|gui|version, install
  api/                  # REST API (M6, отложен)
  core/                 # shim-фасад над execution/pipeline/journal/gate:
                        # API-поверхность для cmd/* и будущих GUI, там живут
                        # интеграционные тесты. Судьба записана в README:
                        # чистка не раньше M6.
  common/               # мелкие помощники (truncate и пр.)
plugins/
  official/             # 5 (созданы ядром: llm-провайдеры, mx, disposable)
  community/            # 14 (community-авторы, волна 2)
registry.yaml           # реестр (19 плагинов + 12 пресетов), в корне
examples/               # 16 пайплайнов-примеров (демо v0.20: when/foreach/parallel)
protocol/               # VERSION (0.2), CHANGELOG, v0.2/PROTOCOL.md
schemas/                # pipeline.v0.2, manifest, request, response
docs/                   # plugin-dev (туториал), quickstart, resume, architecture
archive/                # устаревшие доки (M5/LANDING/POSTS)
var/runs/               # журналы прогонов
```

## Исполнение рана

```
validate (статика) → run:
  secrets preflight → network preflight (deny)
  → [pipeline-foreach: pre-фаза (steps.* источник)]
  → цикл по элементам (или один «элемент» без foreach):
      сегменты шагов: одиночные | parallel_group (goroutine + барьер)
        каждый шаг: when? → [step-foreach: мини-цикл] → runStep
          runStep: manifest → buildInput (bind > from) → subprocess
                   → enforce output → ctx.SetStep / политики on_error
  → post-фаза (after_foreach, агрегаты steps.<id>_all)
  journal.jsonl на каждом переходе; context.json — снапшоты
```

Параллельные ветки работают на **копиях контекста**; слияние после барьера —
в порядке списка шагов (кто быстрее, не влияет на результат).

## Управляющий поток (v0.20, PROTOCOL §12)

- `when:` — условие (строка = «истинно?» или `{path, op, value}`); ложь = `skipped`.
- `foreach:` на шаге — per-item мини-цикл; `steps.<id>_all` — агрегат; stop = стоп рана.
- `parallel_group` — параллельные ветки с барьером; human_gate в группах запрещён.
- Diamond (A,B → C) — из коробки. **Циклы — сознательно НЕ поддерживаются**:
  цикл принадлежит плагину (он сам держит бюджет итераций); ядро блокирует
  циклы в графе (detect-cycle) — решение и причина в PROTOCOL §10.

## Версии

- `VERSION` (корень) — версия приложения, её читает бинарник (CWD в приоритете
  над ldflags).
- `protocol/VERSION` — версия протокола (сейчас 0.2).
- `format_version` в pipeline.yaml и `platform_api` в plugin.yaml — стороны
  контракта.
