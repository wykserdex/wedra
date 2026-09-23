package mcp

// Tool — описание инструмента для tools/list.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

func toolDefs() []Tool {
	obj := func(props map[string]interface{}, required ...string) map[string]interface{} {
		return map[string]interface{}{"type": "object", "properties": props, "required": required}
	}
	str := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	num := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "number", "description": desc}
	}
	return []Tool{
		{
			Name:        "list_plugins",
			Description: "Список плагинов: id, version, description, порты (type/format/optional), permissions",
			InputSchema: obj(map[string]interface{}{"filter": str("подстрока по id/description (опционально)")}),
		},
		{
			Name:        "describe_plugin",
			Description: "Манифест плагина целиком",
			InputSchema: obj(map[string]interface{}{"id": str("id плагина или путь")}, "id"),
		},
		{
			Name:        "validate_pipeline",
			Description: "Проверка пайплайна: {ok, issues[]}. ok:false — нормальный результат, не ошибка",
			InputSchema: obj(map[string]interface{}{
				"yaml": str("YAML пайплайна (или укажите path)"),
				"path": str("путь к YAML в workdir (или укажите yaml)"),
			}),
		},
		{
			Name:        "plan_pipeline",
			Description: "DAG пайплайна: порядок, параллельные группы, фазы",
			InputSchema: obj(map[string]interface{}{
				"yaml": str("YAML пайплайна"),
				"path": str("путь к YAML в workdir (альтернатива yaml)"),
			}),
		},
		{
			Name:        "run_pipeline",
			Description: "Запуск пайплайна: сначала validate, затем {run_id, status}. Решения гейтов через MCP невозможны — статус waiting_human означает «попросите пользователя одобрить в окне wedra»",
			InputSchema: obj(map[string]interface{}{
				"yaml":         str("YAML пайплайна (или path)"),
				"path":         str("путь к YAML в workdir"),
				"wait_seconds": num("сколько секунд ждать завершения (0 — вернуть сразу)"),
			}),
		},
		{
			Name:        "get_run",
			Description: "Статус рана, stats, новые события, выходы шагов (~20KB), pending_gate без токена",
			InputSchema: obj(map[string]interface{}{
				"run_id": str("id рана из run_pipeline"),
				"since":  num("события начиная с индекса (опционально)"),
			}, "run_id"),
		},
		{
			Name:        "cancel_run",
			Description: "Отмена рана (Фаза 5)",
			InputSchema: obj(map[string]interface{}{"run_id": str("id рана")}, "run_id"),
		},
	}
}
