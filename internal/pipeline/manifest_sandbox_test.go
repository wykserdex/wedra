package pipeline

import (
	"strings"
	"testing"
)

func manifestWithSandbox(sandbox string) *Manifest {
	m := &Manifest{
		ID:          "trust",
		Version:     "0.1.0",
		PlatformAPI: "0.1",
		Runtime:     Runtime{Type: "python", Entry: "plugin.py"},
		Input:       map[string]Port{},
		Output:      map[string]Port{},
		Permissions: Permissions{Filesystem: "workspace"},
	}
	if sandbox != "" {
		m.Sandbox = sandbox
	}
	return m
}

func TestManifestSandboxAccepted(t *testing.T) {
	for _, v := range []string{"", SandboxTrusted, SandboxUntrusted} {
		m := manifestWithSandbox(v)
		if err := ValidateManifest(m); err != nil {
			t.Fatalf("sandbox=%q: не должен быть отвергнут: %v", v, err)
		}
		if got := m.Untrusted(); got != (v == SandboxUntrusted) {
			t.Fatalf("sandbox=%q: Untrusted() = %v, ожидалось %v", v, got, v == SandboxUntrusted)
		}
	}
}

func TestManifestSandboxRejectsUnknownValue(t *testing.T) {
	err := ValidateManifest(manifestWithSandbox("sandboxed"))
	if err == nil {
		t.Fatal("sandbox: sandboxed должен отклоняться валидатором")
	}
	if !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("ошибка должна называть поле sandbox, получено: %v", err)
	}
}

func TestManifestUntrustedRejectsSecrets(t *testing.T) {
	m := manifestWithSandbox(SandboxUntrusted)
	m.Permissions.Secrets = []string{"GITHUB_TOKEN"}
	err := ValidateManifest(m)
	if err == nil {
		t.Fatal("untrusted-плагин с permissions.secrets должен отклоняться")
	}
	if !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("ошибка должна объяснять конфликт с изоляцией: %v", err)
	}
}

func TestDecodeManifestAcceptsSandboxField(t *testing.T) {
	raw := []byte("id: trust\nversion: 0.1.0\nplatform_api: \"0.1\"\nruntime:\n  type: python\n  entry: plugin.py\nsandbox: untrusted\ninput: {}\noutput: {}\npermissions:\n  filesystem: workspace\n")
	var m Manifest
	if err := DecodeManifest(raw, &m); err != nil {
		t.Fatalf("поле sandbox должно приниматься декодером: %v", err)
	}
	if !m.Untrusted() {
		t.Fatal("sandbox: untrusted не декодировался")
	}
}
