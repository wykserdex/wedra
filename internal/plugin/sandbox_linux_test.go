//go:build linux

package plugin

import (
	"strings"
	"testing"

	"wedra/internal/pipeline"
)

// Аргументы bwrap проверяются без запуска: на CI-раннерах user namespaces могут
// быть запрещены, и behavioural-тест песочницы там пропускается — форма команды
// всё равно обязана быть верной.

func TestLinuxSandboxArgs(t *testing.T) {
	dir := t.TempDir()
	m := &pipeline.Manifest{
		ID:      "x",
		Runtime: pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:     dir,
	}
	args := sandboxArgsUnchecked(m, []string{"/usr/bin/python3", "/p/plugin.py"})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--die-with-parent",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--ro-bind / /",
		"--proc /proc",
		"--tmpfs " + sandboxScratchPath,
		"--setenv HOME " + sandboxScratchPath,
		"--unshare-net",
		"-- /usr/bin/python3 /p/plugin.py",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("в аргументах bwrap нет %q: %s", want, joined)
		}
	}
	// Регрессия: перекрытие /tmp скрывало бы каталог плагина, установленного
	// во временный каталог, и процесс не смог бы стартовать.
	if strings.Contains(joined, "--tmpfs /tmp") {
		t.Errorf("песочница не должна перекрывать /tmp: %s", joined)
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
	joined := strings.Join(sandboxArgsUnchecked(m, []string{"/usr/bin/python3", "/p/plugin.py"}), " ")
	if strings.Contains(joined, "--unshare-net") {
		t.Error("при объявленной permissions.network netns должен остаться общим")
	}
}
