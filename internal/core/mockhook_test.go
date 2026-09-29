package core

// Имена mock-хуков обязаны совпадать в двух местах: в plugin.test.yaml, где
// env ставится плагину, и в коде плагина, который этот env читает. Расхождение
// тихое — тест просто начинает ходить в сеть (или, наоборот, читает файл там,
// где его не ждут), и при живом CI это видно только по времени прогона.
//
// Опечатка CRTHS_MOCK_FILE была согласована с обеих сторон, поэтому ловля
// именно её — не повод для теста. Тест ловит класс: правку одной стороны.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// mockEnvKeys — ключи env, которые выглядят как mock-хуки. Критерий
// содержания «MOCK», а не точный список: плагин с новым mock-хуком попадёт
// сюда сам.
func mockEnvKeys(t *testing.T, file PluginTestFile) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for _, tc := range file.Tests {
		for k := range tc.Env {
			if strings.Contains(strings.ToUpper(k), "MOCK") {
				keys[k] = true
			}
		}
	}
	return keys
}

func TestPluginMockHooksMatchPluginSource(t *testing.T) {
	repo := coreRepoRoot(t)
	var dirs []string
	for _, group := range []string{"official", "community", "agent-plugins"} {
		entries, err := filepath.Glob(filepath.Join(repo, "plugins", group, "*"))
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range entries {
			if _, err := os.Stat(filepath.Join(dir, "plugin.yaml")); err == nil {
				dirs = append(dirs, dir)
			}
		}
	}
	if len(dirs) == 0 {
		t.Skip("плагины не найдены")
	}

	checked := 0
	for _, dir := range dirs {
		raw, err := os.ReadFile(filepath.Join(dir, "plugin.test.yaml"))
		if err != nil {
			continue
		}
		var file PluginTestFile
		if err := yaml.Unmarshal(raw, &file); err != nil {
			t.Fatalf("%s: %v", filepath.Base(dir), err)
		}
		keys := mockEnvKeys(t, file)
		if len(keys) == 0 {
			continue
		}
		checked++
		source := pluginSource(dir)
		for key := range keys {
			if !strings.Contains(source, key) {
				t.Errorf("%s: plugin.test.yaml ставит env %q, а код плагина такого имени не читает — тест уйдёт в сеть или прочитает не тот файл",
					filepath.Base(dir), key)
			}
		}
	}
	if checked == 0 {
		t.Fatal("ни один плагин не использует mock-хуки — проверка ничего не нашла, а не прошла")
	}
}

// pluginSource — склеенный текст скриптов плагина: mock-режим может читать и
// main.py, и подставной бинарник (mock_holehe.py и подобные).
func pluginSource(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		b.Write(body)
		b.WriteString("\n")
	}
	return b.String()
}

func coreRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "plugins")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("корень репозитория не найден")
		}
		dir = parent
	}
}
