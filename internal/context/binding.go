package context

// Binding — логика резолва путей input.* / steps.* в контексте
// Вынесена из core/runner.go buildInput для переиспользования в pipeline planner

func ResolvePath(path string, data map[string]interface{}) (interface{}, bool) {
	// data — ctx.Data (map с ключами input, steps)
	// path вида input.emails, steps.syntax.syntax
	// используем тот же алгоритм что и Ctx.Get
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, false
	}
	var cur interface{} = data
	for _, p := range parts {
		switch node := cur.(type) {
		case map[string]interface{}:
			v, ok := node[p]
			if !ok {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

func splitPath(path string) []string {
	// простая реализация — split по '.'
	// в будущем — поддержка экранирования
	var res []string
	cur := ""
	for _, ch := range path {
		if ch == '.' {
			if cur != "" {
				res = append(res, cur)
				cur = ""
			}
		} else {
			cur += string(ch)
		}
	}
	if cur != "" {
		res = append(res, cur)
	}
	return res
}
