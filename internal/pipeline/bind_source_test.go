package pipeline

import "testing"

// Регрессия, найденная прогоном пайплайна через MCP: литерал в bind
// (например top_n: 1) не является ссылкой на контекст и не резолвится НИКОГДА.
// Раньше валидация проходила, ран завершался "done", а плагин молча брал
// значение по умолчанию — пайплайн делал не то, что задано в YAML.
func TestBindLiteralSourceIsRejected(t *testing.T) {
	eng := stubBase()
	eng.mans["fake/need_two"] = &Manifest{
		ID: "fake/need_two",
		Input: map[string]Port{
			"a": {Type: "string"},
			"b": {Type: "string", Optional: true},
		},
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:  "lit",
			Input: map[string]interface{}{"x": "v"},
			Steps: []Step{{
				ID:     "s1",
				Plugin: "fake/need_two",
				Bind:   map[string]string{"a": "input.x", "b": "1"},
			}},
		},
	}
	found := false
	for _, is := range ValidateIssues(pf, eng) {
		if is.Code == E_BIND_SOURCE_INVALID {
			found = true
			if is.Severity != SeverityError {
				t.Errorf("E_BIND_SOURCE_INVALID должен быть ошибкой, не %q", is.Severity)
			}
		}
	}
	if !found {
		t.Fatal("литерал в bind не пойман валидатором")
	}
}

func TestBindContextRefAccepted(t *testing.T) {
	for _, src := range []string{"input.text", "steps.s1.out", "steps.s1.out_all"} {
		if !isContextRef(src) {
			t.Errorf("isContextRef(%q) = false, ожидалось true", src)
		}
	}
	for _, src := range []string{"1", "true", "top_n", "", "input", "steps", "/abs/path"} {
		if isContextRef(src) {
			t.Errorf("isContextRef(%q) = true, ожидалось false", src)
		}
	}
}
