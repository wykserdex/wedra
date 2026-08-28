package core

// M5, тестер №3: генератор должен учить не только string-порту —
// при написании первого «серьёзного» плагина приходилось листать PROTOCOL.md,
// чтобы понять, что ждёт валидатор. --example array даёт готовый урок для массивов.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCreateArgs_ExampleFlag(t *testing.T) {
	_, o, err := ParseCreateArgs([]string{"plugins/x", "--example", "array"})
	if err != nil || o.Example != "array" {
		t.Fatalf("opts=%+v err=%v", o, err)
	}
	_, o, err = ParseCreateArgs([]string{"--example=array", "plugins/x"})
	if err != nil || o.Example != "array" {
		t.Fatalf("eq-форма: opts=%+v err=%v", o, err)
	}
}

func TestCreatePlugin_UnknownExampleRejected(t *testing.T) {
	_, err := CreatePluginWith(filepath.Join(t.TempDir(), "x"), CreateOptions{Example: "matrix"})
	if err == nil || !strings.Contains(err.Error(), "string") || !strings.Contains(err.Error(), "array") {
		t.Fatalf("должно перечислить допустимые значения: %v", err)
	}
}

func TestCreatePlugin_ArrayExampleIsGreen(t *testing.T) {
	requirePython(t)
	dir := filepath.Join(t.TempDir(), "plugins", "osint_result_sorter") // ← имя из фидбека
	if _, err := CreatePluginWith(dir, CreateOptions{Example: "array"}); err != nil {
		t.Fatal(err)
	}

	if errs := ValidatePluginDir(dir); len(errs) > 0 {
		t.Fatalf("array-скелет невалиден: %v", errs)
	}
	// критерий генератора: СРАЗУ зелёные контракт-тесты
	passed, failed, err := RunPluginTests(dir, "", false)
	if err != nil || failed != 0 || passed != 3 {
		t.Fatalf("array-скелет: passed=%d failed=%d err=%v", passed, failed, err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	s := string(raw)
	for _, want := range []string{"type: array", "ШПАРГАЛКА", "string | number | boolean | array | object"} {
		if !strings.Contains(s, want) {
			t.Fatalf("в манифесте array-скелета нет %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "TODO") {
		// description/author TODO — это нормально; имеется в виду только отсутствие
		// учебного TODO в зоне input — проверим, что examples не выглядят битыми
		if strings.Contains(s, "items:\n    from: input.items") == false {
			t.Fatalf("array-пример input неожидан:\n%s", s)
		}
	}
}

func TestCreatePlugin_StringExampleHasCheatsheet(t *testing.T) {
	// шпаргалка типов должна быть и в дефолтном шаблоне — иначе боль тестера №3
	// («приходится возвращаться в PROTOCOL.md») закрыта лишь наполовину
	dir := filepath.Join(t.TempDir(), "plain_one")
	if _, err := CreatePlugin(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if !strings.Contains(string(raw), "string | number | boolean | array | object") {
		t.Fatalf("дефолтный шаблон должен содержать шпаргалку типов валидатора:\n%s", raw)
	}
}
