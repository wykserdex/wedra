package plugin

// v0.23: контракт-тесты — типы и форматы проверяются после каждого запуска.

import (
	"strings"
	"testing"

	"wedra/internal/pipeline"
)

func TestKindOf(t *testing.T) {
	cases := []struct {
		v    interface{}
		want string
	}{
		{true, "boolean"},
		{float64(42), "number"},
		{int(7), "number"},
		{"x", "string"},
		{[]interface{}{1}, "array"},
		{map[string]interface{}{"a": 1}, "object"},
		{nil, "null"},
		{struct{ X int }{1}, "unknown"},
	}
	for _, c := range cases {
		if got := KindOf(c.v); got != c.want {
			t.Errorf("KindOf(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestFormatOK(t *testing.T) {
	cases := []struct {
		format, s string
		want      bool
	}{
		{"email", "a@b.c", true},
		{"email", "not-an-email", false},
		{"url", "https://example.com/x", true},
		{"url", "ftp://nope", false},
		{"url", "example.com", false},
		{"ip", "10.0.0.1", true},
		{"ip", "999.1.1.1", false},
		{"file_ref", "/tmp/x", true},
		{"file_ref", "", false},
		{"text", "любой текст", true},
		{"", "x", true},
		{"что-то_неизвестное", "x", true},
	}
	for _, c := range cases {
		if got := FormatOK(c.format, c.s); got != c.want {
			t.Errorf("FormatOK(%q, %q) = %v, want %v", c.format, c.s, got, c.want)
		}
	}
}

func TestEnforceOutputTypeViolation(t *testing.T) {
	// Эксперимент из ревью v0.23: {"total": "НЕ ЧИСЛО"} при type: number
	m := &pipeline.Manifest{
		ID:     "liar",
		Output: map[string]pipeline.Port{"total": {Type: "number"}},
	}
	_, _, err := EnforceOutput(m, map[string]interface{}{"total": "НЕ ЧИСЛО"})
	if err == nil {
		t.Fatal("ожидалось нарушение контракта (string вместо number)")
	}
	if !strings.Contains(err.Error(), "нарушение контракта") {
		t.Fatalf("сообщение не про контракт: %v", err)
	}
}

func TestEnforceOutputFormatViolation(t *testing.T) {
	m := &pipeline.Manifest{
		ID:     "mail",
		Output: map[string]pipeline.Port{"email": {Type: "string", Format: "email"}},
	}
	_, _, err := EnforceOutput(m, map[string]interface{}{"email": "не почта"})
	if err == nil {
		t.Fatal("ожидалось нарушение формата email")
	}
	if !strings.Contains(err.Error(), "формату") {
		t.Fatalf("сообщение не про формат: %v", err)
	}
}

func TestEnforceOutputOK(t *testing.T) {
	m := &pipeline.Manifest{
		ID: "ok",
		Output: map[string]pipeline.Port{
			"total": {Type: "number"},
			"mail":  {Type: "string", Format: "email"},
			"note":  {Type: "string", Optional: true},
		},
	}
	clean, dropped, err := EnforceOutput(m, map[string]interface{}{
		"total": float64(42), "mail": "a@b.c", "лишний": 1,
	})
	if err != nil {
		t.Fatalf("ожидалось ok: %v", err)
	}
	if len(dropped) != 1 || dropped[0] != "лишний" {
		t.Fatalf("лишнее должно отбрасываться: %v", dropped)
	}
	if clean["total"] == nil || clean["note"] != nil {
		t.Fatalf("clean некорректен: %v", clean)
	}
}

func TestEnforceOutputRequiredMissing(t *testing.T) {
	m := &pipeline.Manifest{
		ID:     "req",
		Output: map[string]pipeline.Port{"total": {Type: "number"}},
	}
	_, _, err := EnforceOutput(m, map[string]interface{}{})
	if err == nil {
		t.Fatal("ожидалась ошибка обязательного поля")
	}
}
