package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Контракт резолвинга (v0.1): путь — как есть, голое имя — реестр.
func TestRefRules(t *testing.T) {
	cases := []struct {
		ref       string
		wantLocal bool
	}{
		{"plugins/community/csv_loader", true},
		{"./myplugin", true},
		{"/abs/path", true},
		{"core/human_gate", true},
		{"csv_loader", false},
		{"csv_loader@v0.15", false},
	}
	for _, c := range cases {
		if got := IsLocalRef(c.ref); got != c.wantLocal {
			t.Fatalf("IsLocalRef(%q) = %v, want %v", c.ref, got, c.wantLocal)
		}
	}
}

func TestSplitRef(t *testing.T) {
	if n, v := SplitRef("csv_loader"); n != "csv_loader" || v != "" {
		t.Fatalf("SplitRef: %q %q", n, v)
	}
	if n, v := SplitRef("csv_loader@v0.15"); n != "csv_loader" || v != "v0.15" {
		t.Fatalf("SplitRef: %q %q", n, v)
	}
}

func TestRefToDir(t *testing.T) {
	tmp := t.TempDir()
	pluginsDir := filepath.Join(tmp, "plugins")

	// локальный путь — как есть, без проверок
	d, err := RefToDir("plugins/community/csv_loader", pluginsDir)
	if err != nil || d != "plugins/community/csv_loader" {
		t.Fatalf("local: %q %v", d, err)
	}

	// не установлен
	if _, err := RefToDir("csv_loader", pluginsDir); err == nil {
		t.Fatal("ожидается ошибка «не установлен»")
	}

	// установлен без версии
	writeFile(t, filepath.Join(pluginsDir, "csv_loader", "plugin.yaml"), "id: csv_loader\n")
	d, err = RefToDir("csv_loader", pluginsDir)
	if err != nil || d != filepath.Join(pluginsDir, "csv_loader") {
		t.Fatalf("installed: %q %v", d, err)
	}

	// pinnинг: версия совпадает
	if err := WriteLock(filepath.Join(pluginsDir, "csv_loader"), Lock{Name: "csv_loader", Version: "v0.15"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RefToDir("csv_loader@v0.15", pluginsDir); err != nil {
		t.Fatalf("pin ok: %v", err)
	}
	// pinnинг: версия не совпадает
	if _, err := RefToDir("csv_loader@v0.16", pluginsDir); err == nil {
		t.Fatal("ожидается ошибка «версия не совпадает»")
	}
	// без lock — пин невозможен
	other := filepath.Join(pluginsDir, "other")
	writeFile(t, filepath.Join(other, "plugin.yaml"), "id: other\n")
	if _, err := RefToDir("other@v1", pluginsDir); err == nil {
		t.Fatal("ожидается ошибка «нет lock»")
	}
}

func TestLoadLocal(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, RegistryFile), `
version: "0.1"
plugins:
  csv_loader:
    source: https://example.com/x.git
    path: plugins/community/csv_loader
    description: тест
`)
	h, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	e, ok := h.GetPlugin("csv_loader")
	if !ok {
		t.Fatal("csv_loader не найден")
	}
	if e.Version != "main" { // дефолт version
		t.Fatalf("version: %q", e.Version)
	}
	if _, ok := h.GetPreset("nope"); ok {
		t.Fatal("preset не должен найтись")
	}
	if names := h.PluginNames(); len(names) != 1 || names[0] != "csv_loader" {
		t.Fatalf("names: %v", names)
	}
}

func TestLoadLocalFile(t *testing.T) {
	regDir := t.TempDir()
	raw := "version: \"0.1\"\nplugins:\n  a:\n    source: https://example.com/repo\n    path: plugins/a\n"
	if err := os.WriteFile(filepath.Join(regDir, "registry.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := Load(filepath.Join(regDir, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	e, ok := h.GetPlugin("a")
	if !ok || e.Path != "plugins/a" {
		t.Fatalf("плагин a не найден или неверный path: %+v", e)
	}
	if h.Dir != regDir {
		t.Fatalf("Dir должен быть каталогом файла: %s", h.Dir)
	}
}
