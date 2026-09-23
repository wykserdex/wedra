# wedra mcp — контрактный исполнитель для LLM-агентов

Агент предлагает план → wedra проверяет до запуска → человек одобряет → wedra исполняет и ведёт журнал.

Транспорт: stdio JSON-RPC 2.0, по одному сообщению на строку. Своя минимальная
реализация без зависимостей: `initialize`, `notifications/initialized`,
`tools/list`, `tools/call`, `ping`.

## Запуск

```bash
go build -o wedra ./cmd/wedra
wedra mcp --plugins /abs/path/plugins --workdir /abs/path/project
```

- `--plugins <dir>` — repeatable, корни плагинов (песочница).
- `--workdir <dir>` — корень `path` пайплайнов и `file_ref`.
- Абсолютные пути и `..` вне корней → `E_PLUGIN_OUTSIDE_ROOT`.
- `secrets` — только имена, значений в ответах нет.
- Один ран за раз: занят → `E_RUN_BUSY` с id текущего рана.
- `os.Stdout`/`stdin` изолированы: JSON-RPC — только в proto-stdout,
  весь шум раннера/гейта — в stderr. `StdinUI` в MCP запрещён (`E_NO_GATE_UI`).

## Инструменты

| Tool | Вход | Выход |
|---|---|---|
| `list_plugins` | `filter?` | id, version, description, порты, permissions |
| `describe_plugin` | `id` | манифест целиком |
| `validate_pipeline` | `yaml` \| `path` | `{ok, issues[]}` — `ok:false` норма, не `isError` |
| `plan_pipeline` | `yaml` \| `path` | DAG + issues |
| `run_pipeline` | `yaml`\|`path`, `wait_seconds?` | сначала validate, затем `{run_id, status: running\|waiting_human\|done\|failed}` |
| `get_run` | `run_id`, `since?` | статус, stats, новые события, выходы (~20KB), `pending_gate{step,form,actions}` без токена |
| `cancel_run` | `run_id` | статус (Фаза 5) |

Решения гейтов через MCP **не выставляются никогда**. `get_run` в `waiting_human`
возвращает «попросите пользователя одобрить шаг X в окне wedra».
Для раннов из MCP `AllowAutoApprove=false` всегда, независимо от YAML.

Коды ошибок — `protocol/v0.2/ERRORS.md` (публичный контракт).

## Конфиги клиентов

Claude Desktop (`claude_desktop_config.json`):

```json
{"mcpServers": {"wedra": {"command": "wedra", "args": ["mcp", "--plugins", "/abs/path/plugins", "--workdir", "/abs/path/project"]}}}
```

Claude Code (`.mcp.json` или `--mcp-config`):

```json
{"mcpServers": {"wedra": {"command": "wedra", "args": ["mcp", "--plugins", "/abs/path/plugins", "--workdir", "/abs/path/project"]}}}
```

Cursor (`~/.cursor/mcp.json`):

```json
{"mcpServers": {"wedra": {"command": "wedra", "args": ["mcp", "--plugins", "/abs/path/plugins", "--workdir", "/abs/path/project"]}}}
```

Проверка через MCP Inspector:

```bash
npx @modelcontextprotocol/inspector wedra mcp --plugins ./plugins --workdir .
# initialize → tools/list → validate → run → get_run
```
