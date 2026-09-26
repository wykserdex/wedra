//go:build linux

package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	scratch := t.TempDir()
	args := sandboxArgsUnchecked(m, []string{"/usr/bin/python3", "/p/plugin.py"}, scratch)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--die-with-parent",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--ro-bind / /",
		"--proc /proc",
		"--bind " + resolveSandboxPath(scratch) + " " + resolveSandboxPath(scratch),
		"--setenv HOME " + resolveSandboxPath(scratch),
		"--unshare-net",
		"-- /usr/bin/python3 /p/plugin.py",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("в аргументах bwrap нет %q: %s", want, joined)
		}
	}
	// Регрессия: перекрытие /tmp скрывало бы каталог плагина, установленного
	// во временный каталог, и процесс не смог бы стартовать. Точка монтирования
	// также должна существовать заранее: после --ro-bind / / создать её нельзя.
	if strings.Contains(joined, "--tmpfs") {
		t.Errorf("песочница не должна использовать tmpfs: %s", joined)
	}
	if _, err := os.Stat(resolveSandboxPath(scratch)); err != nil {
		t.Fatalf("scratch-каталог должен существовать до монтирования: %v", err)
	}
	// Каталог плагина не должен пробрасываться на запись.
	if strings.Contains(joined, "--bind "+resolveSandboxPath(dir)) {
		t.Errorf("каталог плагина не должен быть rw: %s", joined)
	}
}

// TestLinuxSandboxBlocksPluginDirWrite — security-регрессия: каталог плагина
// должен быть read-only, иначе внешний код может модифицировать себя и
// закрепиться на диске между запусками.
func TestLinuxSandboxBlocksPluginDirWrite(t *testing.T) {
	if !sandboxUsable() {
		t.Skip("песочница недоступна на этом хосте")
	}
	requirePython(t)
	dir := t.TempDir()
	script := "import os, sys\n" +
		"here = os.path.dirname(os.path.abspath(__file__))\n" +
		"try:\n" +
		"    open(os.path.join(here, 'selfmod.py'), 'w').write('x')\n" +
		"except OSError:\n" +
		"    sys.stdout.write('{\"status\":\"ok\",\"output\":{\"plugin_dir_readonly\": true}}')\n" +
		"else:\n" +
		"    sys.exit(1)\n"
	m := writePlugin(t, dir, script)

	res := ExecWithEnvCtx(AllowUntrustedPlugins(context.Background()), m, []byte("{}"), 30*time.Second, nil)
	if !res.OK() {
		t.Fatalf("плагин не должен получать запись в свой каталог: %+v (stderr: %s)", res, res.Stderr)
	}
	if ro, _ := res.Output["plugin_dir_readonly"].(bool); !ro {
		t.Fatal("песочница не заблокировала запись в каталог плагина")
	}
	if _, err := os.Stat(filepath.Join(dir, "selfmod.py")); err == nil {
		t.Fatal("плагин смог создать файл в своём каталоге на хосте")
	}
}

func TestLinuxSandboxKeepsNetworkWhenDeclared(t *testing.T) {
	m := &pipeline.Manifest{
		ID:          "x",
		Runtime:     pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:         t.TempDir(),
		Permissions: pipeline.Permissions{Network: []pipeline.NetworkPermission{{Host: "api.example.com", Port: 443}}},
	}
	joined := strings.Join(sandboxArgsUnchecked(m, []string{"/usr/bin/python3", "/p/plugin.py"}, t.TempDir()), " ")
	if strings.Contains(joined, "--unshare-net") {
		t.Error("при объявленной permissions.network netns должен остаться общим")
	}
}
