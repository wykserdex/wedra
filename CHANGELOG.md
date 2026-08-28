# Changelog — честная 0.x

## v0.11 (2026-08-28) — M6 GUI scaffold
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
