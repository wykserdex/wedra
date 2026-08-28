package core

// Тесты генератора скелета. Главный критерий: свежесгенерированный плагин
// обязан проходить validate + test без единой правки.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatePluginSkeletonIsGreen(t *testing.T) {
	requirePython(t)
	dir := filepath.Join(t.TempDir(), "word_counter")

	id, err := CreatePlugin(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != "word_counter" {
		t.Fatalf("id = %q", id)
	}

	for _, f := range []string{"plugin.yaml", "main.py", "plugin.test.yaml", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("нет файла %s: %v", f, err)
		}
	}

	if errs := ValidatePluginDir(dir); len(errs) != 0 {
		t.Fatalf("сгенерированный манифест невалиден: %v", errs)
	}

	passed, failed, err := RunPluginTests(dir, "", true)
	if err != nil {
		t.Fatalf("стартовые тесты не запустились: %v", err)
	}
	if failed != 0 || passed != 2 {
		t.Fatalf("скелет должен быть зелёным из коробки: passed=%d failed=%d", passed, failed)
	}
}

func TestCreatePluginTeachesProtocol(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo_plugin")
	if _, err := CreatePlugin(dir); err != nil {
		t.Fatal(err)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.py"))
	text := string(main)
	// скелет — проигрываемый урок: конверт, классы exit-кодов, граница ответственности
	for _, fragment := range []string{`"status": "ok"`, `"status": "error"`, "retryable", "exit 2"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("в шаблоне нет фрагмента-урока %q", fragment)
		}
	}
}

func TestCreatePluginRejectsBadID(t *testing.T) {
	for _, bad := range []string{"My-Plugin", "1st", "has space", "кириллица"} {
		if _, err := CreatePlugin(filepath.Join(t.TempDir(), bad)); err == nil {
			t.Fatalf("id %q должен быть отклонён", bad)
		}
	}
}

func TestCreatePluginRefusesNonEmptyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "taken_dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("# чужое"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlugin(dir); err == nil {
		t.Fatal("генератор обязан отказаться перезаписывать непустую папку")
	}
	// ...но пустая папка — ок
	empty := filepath.Join(t.TempDir(), "empty_dir")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlugin(empty); err != nil {
		t.Fatalf("пустая существующая папка должна быть ок: %v", err)
	}
}

func TestCreatePluginNestedPath(t *testing.T) {
	requirePython(t)
	// id берётся из имени папки, путь может быть вложенным
	dir := filepath.Join(t.TempDir(), "plugins", "pack_a", "my_new_tool")
	id, err := CreatePlugin(dir)
	if err != nil || id != "my_new_tool" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if passed, failed, _ := RunPluginTests(dir, "", true); failed != 0 || passed != 2 {
		t.Fatalf("passed=%d failed=%d", passed, failed)
	}
}
