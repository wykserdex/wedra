package core

// Тесты на `tool plugin test`: матчеры — юнитно, прогон — на фикстурах
// и (интеграционно) на всех шипленных плагинах репозитория.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── матчеры (чистые функции) ────────────────────────────────────────────

func TestMatchValueDeepEqual(t *testing.T) {
	var failures []string
	fail := func(f string, a ...interface{}) { failures = append(failures, f) }

	matchValue("x", 42, 42.0, fail) // YAML int ≡ JSON float64
	if len(failures) != 0 {
		t.Fatalf("int/float должны сойтись: %v", failures)
	}
	matchValue("x", "abc", "abd", fail)
	if len(failures) != 1 {
		t.Fatal("разные строки должны давать провал")
	}
	matchValue("x",
		map[string]interface{}{"a": []interface{}{1.0, 2.0}},
		map[string]interface{}{"a": []interface{}{1.0, 2.0}}, fail)
	if len(failures) != 1 {
		t.Fatalf("глубокое равенство вложенных структур: %v", failures)
	}
}

func TestMatchValueMatchers(t *testing.T) {
	var failures []string
	fail := func(f string, a ...interface{}) { failures = append(failures, f) }

	matchValue("x", map[string]interface{}{"contains": "mock"}, "[mock:x] t", fail)
	matchValue("x", map[string]interface{}{"type": "array"}, []interface{}{}, fail)
	matchValue("x", map[string]interface{}{"type": "string"}, 42.0, fail) // провал
	matchValue("x", map[string]interface{}{"contains": "q"}, 42.0, fail)   // ни строка, ни массив = провал
	matchValue("x", map[string]interface{}{"equals": true}, true, fail)
	if len(failures) != 2 {
		t.Fatalf("ожидалось 2 провала (type, contains-not-str), got %d: %v", len(failures), failures)
	}
}

// contains по массивам — фикс по фидбеку тестера №1
// (раньше «массив не содержит» резал глаз, когда элемент там был).
func TestMatchContainsArrays(t *testing.T) {
	var failures []string
	fail := func(f string, a ...interface{}) { failures = append(failures, f) }

	arr := []interface{}{"syntax_bad", "host_is_ip", "disposable"}
	matchValue("reasons", map[string]interface{}{"contains": "host_is_ip"}, arr, fail)
	if len(failures) != 0 {
		t.Fatalf("существующий элемент должен находиться: %v", failures)
	}

	matchValue("reasons", map[string]interface{}{"contains": "no_such"}, arr, fail)
	if len(failures) != 1 {
		t.Fatal("отсутствующий элемент должен давать провал")
	}
	if !strings.Contains(failures[0], "массив не содержит элемент") {
		t.Fatalf("сообщение должно говорить про элемент массива: %q", failures[0])
	}

	// числовые элементы — через глубокое сравнение
	failures = nil
	matchValue("nums", map[string]interface{}{"contains": 2}, []interface{}{1.0, 2.0, 3.0}, fail)
	if len(failures) != 0 {
		t.Fatalf("YAML int 2 должен совпасть с JSON 1.0,2.0,3.0: %v", failures)
	}
}

// Runtime-проверка типов по манифесту: плагин вернул number там,
// где output объявлен string → contract-тест обязан упасть.
func TestPluginTestCatchesTypeDrift(t *testing.T) {
	requirePython(t)
	passed, failed, err := RunPluginTests("testdata/plugins/type_drifter", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 || passed != 0 {
		t.Fatalf("дрейф типа должен фейлить кейс: passed=%d failed=%d", passed, failed)
	}
}

// Позитивный путь type-check: строка от плагина, объявленная string — проходит.
func TestPluginTestTypeCheckPassesHonestPlugin(t *testing.T) {
	requirePython(t)
	passed, failed, err := RunPluginTests("testdata/plugins/echo_ok", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 0 || passed == 0 {
		t.Fatalf("честный плагин не должен фейлить type-check: passed=%d failed=%d", passed, failed)
	}
}

// ── прогон на фикстурах ─────────────────────────────────────────────────

func TestPluginTestFixturesPass(t *testing.T) {
	requirePython(t)
	for _, name := range []string{"echo_ok", "failer"} {
		passed, failed, err := RunPluginTests("testdata/plugins/"+name, "", true)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if failed != 0 || passed == 0 {
			t.Fatalf("%s: passed=%d failed=%d", name, passed, failed)
		}
	}
}

func TestPluginTestDetectsFailure(t *testing.T) {
	requirePython(t)
	// спека с заведомо неверным ожиданием
	spec := "tests:\n  - name: ловушка\n    input: { value: \"hi\" }\n    expect: { status: ok, output: { value: \"WRONG\" } }\n"
	specPath := filepath.Join(t.TempDir(), "bad.test.yaml")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, failed, err := RunPluginTests("testdata/plugins/echo_ok", specPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if passed != 0 || failed != 1 {
		t.Fatalf("заведомо битое ожидание должно фейлиться: passed=%d failed=%d", passed, failed)
	}
}

func TestPluginTestNoSpecFile(t *testing.T) {
	if _, _, err := RunPluginTests("testdata/plugins/crasher", "", true); err == nil {
		t.Fatal("отсутствующий plugin.test.yaml обязан быть ошибкой с подсказкой")
	}
}

// ── интеграция: все шипленные плагины проходят свои plugin.test.yaml ────

func TestPluginTestShippedPlugins(t *testing.T) {
	requirePython(t)
	dirs, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatal("не найдены шипленные плагины")
	}
	for _, dir := range dirs {
		passed, failed, err := RunPluginTests(dir, "", true)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		if failed != 0 {
			t.Fatalf("%s: %d тестов упали (passed=%d)", dir, failed, passed)
		}
	}
}
