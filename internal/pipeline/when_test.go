package pipeline

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func whenData() map[string]interface{} {
	return map[string]interface{}{
		"input": map[string]interface{}{
			"n":     float64(15),
			"empty": "",
			"arr":   []interface{}{"a", "b"},
			"obj":   map[string]interface{}{"x": 1},
			"zero":  float64(0),
			"neg":   float64(-3),
			"word":  "привет мир",
			"names": []interface{}{"ann", "bob"},
		},
		"steps": map[string]interface{}{
			"stats": map[string]interface{}{
				"words":  float64(42),
				"ok":     true,
				"empty":  "",
				"absent": nil,
			},
		},
	}
}

func eval(t *testing.T, w When) (bool, error) {
	t.Helper()
	ok, err := EvaluateWhen(w, whenData())
	if err != nil && ok {
		t.Fatalf("ok=true при ошибке: %v", err)
	}
	return ok, err
}

func TestWhenTruthy(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"input.n", true},
		{"input.zero", false},
		{"input.neg", true},
		{"input.empty", false},
		{"input.arr", true},
		{"input.obj", true},
		{"steps.stats.ok", true},
		{"input.no_such", false},
	}
	for _, c := range cases {
		got, err := eval(t, When{Path: c.path, Op: OpTruthy})
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.path, got, c.want)
		}
	}
}

func TestWhenExistsMissing(t *testing.T) {
	ok, _ := eval(t, When{Path: "steps.stats.words", Op: OpExists})
	if !ok {
		t.Error("exists: путь есть")
	}
	ok, _ = eval(t, When{Path: "input.no_such", Op: OpExists})
	if ok {
		t.Error("exists: пути нет")
	}
	ok, _ = eval(t, When{Path: "input.no_such", Op: OpMissing})
	if !ok {
		t.Error("missing: пути нет → true")
	}
	ok, _ = eval(t, When{Path: "steps.stats.words", Op: OpMissing})
	if ok {
		t.Error("missing: путь есть → false")
	}
}

func TestWhenEqNeq(t *testing.T) {
	// int из YAML == float64 из JSON
	ok, err := eval(t, When{Path: "input.n", Op: OpEq, Value: 15})
	if err != nil || !ok {
		t.Errorf("eq 15: %v %v", ok, err)
	}
	ok, _ = eval(t, When{Path: "input.n", Op: OpEq, Value: 16})
	if ok {
		t.Error("eq 16: должно быть false")
	}
	ok, _ = eval(t, When{Path: "input.n", Op: OpNeq, Value: 16})
	if !ok {
		t.Error("neq 16: должно быть true")
	}
	// строки
	ok, _ = eval(t, When{Path: "input.word", Op: OpEq, Value: "привет мир"})
	if !ok {
		t.Error("eq строки")
	}
	// null: нет пути == null
	ok, _ = eval(t, When{Path: "input.no_such", Op: OpEq, Value: nil})
	if !ok {
		t.Error("eq null для отсутствующего пути")
	}
	// числа != строки
	ok, _ = eval(t, When{Path: "input.n", Op: OpEq, Value: "15"})
	if ok {
		t.Error("eq number vs string: false")
	}
}

func TestWhenNumeric(t *testing.T) {
	cases := []struct {
		op    string
		value interface{}
		want  bool
	}{
		{"gt", 10, true},
		{"gt", 42, false},
		{"gte", 42, true},
		{"lt", 100, true},
		{"lte", 14, false},
	}
	for _, c := range cases {
		ok, err := eval(t, When{Path: "steps.stats.words", Op: c.op, Value: c.value})
		if err != nil {
			t.Fatalf("%s: %v", c.op, err)
		}
		if ok != c.want {
			t.Errorf("words %s %v: got %v want %v", c.op, c.value, ok, c.want)
		}
	}
	// не-число → ошибка
	_, err := eval(t, When{Path: "input.word", Op: OpGt, Value: 1})
	if err == nil {
		t.Error("gt по строке: ожидается ошибка")
	}
}

func TestWhenContains(t *testing.T) {
	ok, _ := eval(t, When{Path: "input.word", Op: OpContains, Value: "мир"})
	if !ok {
		t.Error("contains: подстрока есть")
	}
	ok, _ = eval(t, When{Path: "input.names", Op: OpContains, Value: "bob"})
	if !ok {
		t.Error("contains: элемент массива")
	}
	ok, _ = eval(t, When{Path: "input.names", Op: OpContains, Value: "zoe"})
	if ok {
		t.Error("contains: элемента нет")
	}
}

func TestWhenUnmarshal(t *testing.T) {
	// строковый формат
	var s struct {
		When When `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte("when: steps.stats.ok"), &s); err != nil {
		t.Fatal(err)
	}
	if s.When.Op != OpTruthy || s.When.Path != "steps.stats.ok" {
		t.Errorf("строковый when: %+v", s.When)
	}
	// объектный формат, op по умолчанию eq
	if err := yaml.Unmarshal([]byte("when: { path: input.n, value: 15 }"), &s); err != nil {
		t.Fatal(err)
	}
	if s.When.Op != OpEq || s.When.Value != 15 {
		t.Errorf("объектный when: %+v", s.When)
	}
	// null when = условия нет (ноль-значение, IsSet()=false)
	var n struct {
		When When `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte("when: ~"), &n); err != nil {
		t.Fatal(err)
	}
	if n.When.IsSet() {
		t.Error("null when: должно выглядеть как «условия нет»")
	}
	// пустая строка → ошибка
	if err := yaml.Unmarshal([]byte("when: ''"), &s); err == nil {
		t.Error("пустой when: ожидается ошибка")
	}
}
