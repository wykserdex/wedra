package context

import "strings"

// Ctx — shared context: {"input": {...}, "steps": {"<step_id>": {...}}}.
type Ctx struct {
	Data map[string]interface{}
}

func NewCtx(input map[string]interface{}) *Ctx {
	if input == nil {
		input = map[string]interface{}{}
	}
	return &Ctx{Data: map[string]interface{}{
		"input": input,
		"steps": map[string]interface{}{},
	}}
}

func (c *Ctx) ResetSteps() {
	c.Data["steps"] = map[string]interface{}{}
}

func (c *Ctx) SetInput(key string, val interface{}) {
	c.Data["input"].(map[string]interface{})[key] = val
}

func (c *Ctx) SetStep(stepID string, out map[string]interface{}) {
	c.Data["steps"].(map[string]interface{})[stepID] = out
}

// Get разрешает dot-путь: "input.emails", "steps.syntax.mx".
func (c *Ctx) Get(path string) (interface{}, bool) {
	var cur interface{} = c.Data
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}
