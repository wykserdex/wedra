//go:build linux

package plugin

import (
	"strings"
	"testing"

	"wedra/internal/pipeline"
)

func TestLinuxSandboxArgs(t *testing.T) {
	dir := t.TempDir()
	m := &pipeline.Manifest{
		ID:      "x",
		Runtime: pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:     dir,
	}
	_, args, err := sandboxArgs(m, []string{"/usr/bin/python3", "/p/plugin.py"})
	if err != nil {
		t.Skipf("песочница недоступна: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--die-with-parent",
		"--unshare-pid",
		"--ro-bind / /",
		"--tmpfs /tmp",
		"--setenv HOME /tmp",
		"--unshare-net",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("в аргументах bwrap нет %q: %s", want, joined)
		}
	}
	// Каталог плагина не должен пробрасываться на запись.
	if strings.Contains(joined, "--bind "+resolveSandboxPath(dir)) {
		t.Errorf("каталог плагина не должен быть rw: %s", joined)
	}
}

func TestLinuxSandboxKeepsNetworkWhenDeclared(t *testing.T) {
	m := &pipeline.Manifest{
		ID:          "x",
		Runtime:     pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:         t.TempDir(),
		Permissions: pipeline.Permissions{Network: []pipeline.NetworkPermission{{Host: "api.example.com", Port: 443}}},
	}
	_, args, err := sandboxArgs(m, []string{"/usr/bin/python3", "/p/plugin.py"})
	if err != nil {
		t.Skipf("песочница недоступна: %v", err)
	}
	if strings.Contains(strings.Join(args, " "), "--unshare-net") {
		t.Error("при объявленной permissions.network netns должен остаться общим")
	}
}
