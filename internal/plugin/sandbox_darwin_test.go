//go:build darwin

package plugin

import (
	"strings"
	"testing"

	"wedra/internal/pipeline"
)

func TestDarwinProfileResolvesSymlinks(t *testing.T) {
	// $TMPDIR на macOS — /var/folders/... -> /private/var/folders/...;
	// профиль обязан содержать резолвнутый путь, иначе запись запрещается.
	dir := t.TempDir()
	resolved := resolveSandboxPath(dir)
	profile := sandboxProfile(resolved, resolved, false)
	if !strings.Contains(profile, resolved) {
		t.Fatalf("профиль не содержит резолвнутый путь %q: %s", resolved, profile)
	}
	if !strings.Contains(profile, "(deny file-write*)") {
		t.Errorf("профиль должен запрещать запись по умолчанию: %s", profile)
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Errorf("без объявленной сети сеть должна быть запрещена: %s", profile)
	}
}

func TestDarwinSandboxArgs(t *testing.T) {
	dir := t.TempDir()
	m := &pipeline.Manifest{
		ID:      "x",
		Runtime: pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:     dir,
	}
	launcher, args, err := sandboxArgs(m, []string{"/usr/bin/python3", "/p/plugin.py"})
	if err != nil {
		t.Skipf("песочница недоступна: %v", err)
	}
	if !strings.HasSuffix(launcher, "sandbox-exec") {
		t.Errorf("launcher = %q, ожидался sandbox-exec", launcher)
	}
	if args[0] != "-p" {
		t.Fatalf("первый аргумент sandbox-exec должен быть -p, получено %q", args[0])
	}
	if args[len(args)-1] != "/p/plugin.py" {
		t.Errorf("команда плагина должна быть последней: %v", args)
	}
}
