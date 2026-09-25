package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDoPluginInstallRejectsUnsafeName(t *testing.T) {
	if err := doPluginInstall("../../escape", "", t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("unsafe plugin name was accepted")
	}
}

func TestDoPluginInstallKeepsOldVersionOnValidationFailure(t *testing.T) {
	source := t.TempDir()
	pluginDir := filepath.Join(source, "plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: safe\nversion: 0.1.0\nplatform_api: \"0.1\"\nruntime:\n  type: python\n  entry: main.py\ninput: {}\noutput: {}\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte("pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	registryDir := t.TempDir()
	registryYAML := "version: \"0.1\"\nplugins:\n  safe:\n    source: " + strconv.Quote(source) + "\n    path: plugin\n    version: main\n"
	if err := os.WriteFile(filepath.Join(registryDir, "registry.yaml"), []byte(registryYAML), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "plugins")
	oldDir := filepath.Join(dest, "safe")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(oldDir, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := doPluginInstall("safe", "", registryDir, dest); err == nil {
		t.Fatal("invalid plugin was installed")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("old installation was removed: %v", err)
	}
}
