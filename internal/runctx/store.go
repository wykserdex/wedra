package runctx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Ctx — shared context: {"input": {...}, "steps": {"<step_id>": {...}}}.
type Ctx struct {
	Data map[string]interface{}
}

// ShapeError — контекст не той формы: поле (input/steps) не объект.
// errors.As на границе доверия (--resume читает context.json с диска).
type ShapeError struct {
	Field string
	Got   interface{}
}

func (e *ShapeError) Error() string {
	return fmt.Sprintf("%q должен быть object, получено %s", e.Field, jsonKind(e.Got))
}

// jsonKind — имя JSON-типа значения (для сообщения об ошибке формы).
func jsonKind(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64, float32, int, int64, json.Number:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	}
	return "unknown"
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

// Normalize — проверка формы контекста на границе доверия. context.json
// приходит с диска, поэтому input и steps обязаны быть объектами, если
// присутствуют; nil/отсутствие — пустая карта (нормализуется, не ошибка).
// Любая другая форма (steps: 5, input: []) — *ShapeError вместо паники в
// accessors. Валидный контекст возвращается как есть: нормальный resume
// не меняется.
func Normalize(data map[string]interface{}) (map[string]interface{}, error) {
	if data == nil {
		data = map[string]interface{}{}
	}
	for _, ns := range []string{"input", "steps"} {
		raw, present := data[ns]
		if !present || raw == nil {
			data[ns] = map[string]interface{}{}
			continue
		}
		if _, ok := raw.(map[string]interface{}); !ok {
			return nil, &ShapeError{Field: ns, Got: raw}
		}
	}
	return data, nil
}

// namespace — карта namespace (input/steps) с починкой формы: не-объект
// заменяется пустой картой. Accessors не паникуют на битом/чужом контексте.
func (c *Ctx) namespace(key string) map[string]interface{} {
	if c.Data == nil {
		c.Data = map[string]interface{}{}
	}
	if m, ok := c.Data[key].(map[string]interface{}); ok {
		return m
	}
	m := map[string]interface{}{}
	c.Data[key] = m
	return m
}

func (c *Ctx) ResetSteps() {
	if c == nil {
		return
	}
	if c.Data == nil {
		c.Data = map[string]interface{}{}
	}
	c.Data["steps"] = map[string]interface{}{}
}

func (c *Ctx) SetInput(key string, val interface{}) {
	if c == nil {
		return
	}
	c.namespace("input")[key] = val
}

func (c *Ctx) SetStep(stepID string, out map[string]interface{}) {
	if c == nil {
		return
	}
	c.namespace("steps")[stepID] = out
}

// Input — карта input; создаётся, если её нет или значение не объект
// (nil-контекст даёт nil, читать из него безопасно).
func (c *Ctx) Input() map[string]interface{} {
	if c == nil {
		return nil
	}
	return c.namespace("input")
}

// Steps — карта steps; создаётся, если её нет или значение не объект.
func (c *Ctx) Steps() map[string]interface{} {
	if c == nil {
		return nil
	}
	return c.namespace("steps")
}

// Get разрешает dot-путь: "input.emails", "steps.syntax.mx".
func (c *Ctx) Get(path string) (interface{}, bool) {
	if c == nil || c.Data == nil {
		return nil, false
	}
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
