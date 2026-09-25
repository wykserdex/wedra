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

func TestRefToDirNestedFallback(t *testing.T) {
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	nested := filepath.Join(pluginsDir, "community", "csv_loader")
	writeFile(t, filepath.Join(nested, "plugin.yaml"), "id: csv_loader\n")
	got, err := RefToDir("csv_loader", pluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != nested {
		t.Fatalf("got %q, want %q", got, nested)
	}
}

func TestNormalizePluginRef(t *testing.T) {
	name, version, ok := NormalizePluginRef(`plugins\community\csv_loader@v0.8a`)
	if !ok || name != "csv_loader" || version != "v0.8a" {
		t.Fatalf("got %q %q %v", name, version, ok)
	}
	if _, _, ok := NormalizePluginRef("plugins/community/a/b"); ok {
		t.Fatal("nested path должен быть отвергнут")
	}
}

func TestRegistryRejectsEscapingPath(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, RegistryFile), "version: \"0.1\"\nplugins:\n  bad:\n    source: x\n    path: ../outside\n")
	if _, err := Load(tmp); err == nil {
		t.Fatal("escaping registry path должен быть отвергнут")
	}
}

func TestRefToDirRejectsReservedBuiltin(t *testing.T) {
	for _, ref := range []string{"core/does_not_exist", "core/human_gate/extra", "core"} {
		if _, err := RefToDir(ref, t.TempDir()); err == nil {
			t.Fatalf("reserved builtin %q was accepted", ref)
		}
	}
}

func TestRegistryRejectsUnsafeNamesAndCommits(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, RegistryFile), "version: \"0.1\"\nplugins:\n  ../../escape:\n    source: x\n    path: .\n")
	if _, err := Load(tmp); err == nil {
		t.Fatal("unsafe registry name was accepted")
	}
	writeFile(t, filepath.Join(tmp, RegistryFile), "version: \"0.1\"\nplugins:\n  good:\n    source: x\n    path: .\n    commit: short\n")
	if _, err := Load(tmp); err == nil {
		t.Fatal("short commit was accepted")
	}
}

func TestValidateComponent(t *testing.T) {
	for _, name := range []string{"plugin", "plugin-1", "plugin.name"} {
		if err := ValidateComponent(name); err != nil {
			t.Fatalf("valid component %q: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "../x", `..\\x`, "C:x", "x/y"} {
		if err := ValidateComponent(name); err == nil {
			t.Fatalf("unsafe component accepted: %q", name)
		}
	}
}

func TestRegistryEntryRequiresRemotePin(t *testing.T) {
	entry := Entry{Source: "https://example.com/repo.git", Path: ".", Version: "main"}
	if err := ValidateEntry(entry, true); err == nil {
		t.Fatal("remote registry entry without commit pin was accepted")
	}
	entry.Commit = "0123456789abcdef0123456789abcdef01234567"
	if err := ValidateEntry(entry, true); err != nil {
		t.Fatalf("valid pinned entry rejected: %v", err)
	}
}

func TestRegistryRejectsUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, RegistryFile), "version: \"0.1\"\nunknown: true\nplugins: {}\n")
	if _, err := Load(tmp); err == nil {
		t.Fatal("unknown registry field was accepted")
	}
}
