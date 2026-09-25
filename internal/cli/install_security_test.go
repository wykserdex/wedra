package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func useWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestInstallPipelinePresetDoesNotPublishInvalidPreset(t *testing.T) {
	root := t.TempDir()
	useWorkingDir(t, root)
	raw := []byte(`format_version: "0.2"
pipeline:
  name: invalid
  gates: typo
  steps:
    - id: review
      plugin: core/human_gate
`)
	if _, err := installPipelinePreset(raw, "invalid", "", presetProvenance{}); err == nil {
		t.Fatal("invalid preset was accepted")
	}
	if _, err := os.Stat(filepath.Join("examples", "invalid.yaml")); !os.IsNotExist(err) {
		t.Fatalf("invalid preset became visible: %v", err)
	}
}

func TestInstallPipelinePresetPreservesVersionPin(t *testing.T) {
	root := t.TempDir()
	useWorkingDir(t, root)
	source := filepath.Join(root, "source")
	pluginDir := filepath.Join(source, "plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: foo
version: 0.1.0
platform_api: "0.1"
runtime:
  type: python
  entry: main.py
input:
  text:
    from: input.text
    type: string
output:
  value:
    type: string
permissions:
  network: []
  filesystem: none
  secrets: []
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte("print('{\"status\":\"ok\",\"output\":{\"value\":\"ok\"}}')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	registryYAML := "version: \"0.1\"\nplugins:\n  foo:\n    source: " + strconv.Quote(source) + "\n    path: plugin\n    version: v1\n"
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(registryYAML), 0644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`format_version: "0.2"
pipeline:
  name: pinned
  input:
    text: hello
  steps:
    - id: foo
      plugin: plugins/community/foo@v1
      bind:
        text: input.text
`)
	result, err := installPipelinePreset(raw, "pinned", filepath.Join(root, "registry.yaml"), presetProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Installed != 1 {
		t.Fatalf("installed=%d, want 1", result.Installed)
	}
	normalized, err := os.ReadFile(result.OutFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(normalized), "plugin: foo@v1") {
		t.Fatalf("version pin lost:\n%s", normalized)
	}
}
