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

- `--plugins <dir>` — repeatable, корни плагинов.
- `--workdir <dir>` — корень `path` пайплайнов и `file_ref`.
- Это проверка ссылок и путей, не OS-песочница: процессы плагинов наследуют права пользователя.
- Абсолютные пути и `..` вне корней → `E_PLUGIN_OUTSIDE_ROOT`.
- `secrets` — только имена, значений в ответах нет.
- Один ран за раз: занят → `E_RUN_BUSY` с id текущего рана.
- `--runs-dir <dir>` — каталог ранов (по умолчанию `<workdir>/var/runs`).
- Флаги принимают и `--x v`, и `--x=v`.
- `os.Stdout`/`stdin` изолированы: JSON-RPC — только в proto-stdout,
  весь шум раннера/гейта — в stderr. `StdinUI` в MCP запрещён (`E_NO_GATE_UI`).

## Инструменты

| Tool | Вход | Выход |
|---|---|---|
| `list_plugins` | `filter?` | id, version, description, порты, permissions |
| `describe_plugin` | `id` | манифест целиком |
| `validate_pipeline` | `yaml` \| `path` | `{ok, issues[]}` — `ok:false` норма, не `isError` |
| `plan_pipeline` | `yaml` \| `path` | DAG + issues |
| `run_pipeline` | `yaml`\|`path`, `wait_seconds?` | сначала validate, затем `{run_id, status: running\|waiting_human\|done\|failed\|cancelled}` |
| `get_run` | `run_id`, `since?` | статус, stats, новые события, выходы (~20KB), `pending_gate{step,form,actions}` без токена |
| `cancel_run` | `run_id` | `{status:"cancelling"}`; затем `get_run` → `cancelled` (`E_RUN_DONE`, если ран уже завершён) |

## Гейты: как человек одобряет шаг агента

`wedra mcp` поднимает **консоль гейтов** — HTTP на `127.0.0.1:<случайный порт>`
(`--gui-listen=host:port`) с сессией человека (ключ 128 бит, живёт только
в памяти процесса).

1. Ран агента доходит до `human_gate` → `get_run` отдаёт `waiting_human` +
   `pending_gate{step,form,actions}`.
2. wedra открывает браузер человека на `/?run=<id>&k=<ключ>` (один раз на
   ран). Ключ обменивается на HttpOnly cookie, после чего redirect без `k`.
   С `--no-open` или `--print-link` ссылка с ключом печатается в stderr MCP-процесса
   (лог клиента).
3. Агент в `hint` видит только адрес `/?run=<id>` **без ключа**. Его
   `curl …/gate` → `401 E_SESSION_REQUIRED`.
4. Человек правит форму, жмёт accept или reject. В журнал пишется
   `gate_decision{source:"gui", session:<хэш>}`, ран продолжается.

`--no-gui`: консоли нет. Пайплайн с шагом-гейтом отклоняется **до старта**
(`E_NO_HUMAN_CHANNEL`), чтобы ран не висел вечно в `waiting_human`.

Модель угроз: ключ защищает от агента и любых процессов, которые умеют только
слать HTTP на локальный порт. Он не защищает от процесса, который читает
stderr MCP-клиента, экран или профиль браузера того же пользователя. Для
этого нужна отдельная ОС-учётка или `wedra approve` в TTY.

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
