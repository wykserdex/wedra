package registry

// v0.28a: supply-chain пин — install сверяет HEAD клона с commit из реестра.
// Тег переставили → CloneToPinned падает (fail-closed), не ставит чужой код.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %s", args, err, out)
	}
	return string(out)
}

// pinRepo — локальный git-репо (file://): коммит A, тег v1 → A, коммит B,
// (опц.) тег v1 двигают на B. Возвращает source (file://) и SHA A/B.
func pinRepo(t *testing.T, moveTag bool) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.email", "pin@test")
	git(t, dir, "config", "user.name", "pin")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("A\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "A")
	shaA := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "tag", "v1")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("B\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "B")
	shaB := git(t, dir, "rev-parse", "HEAD")
	if moveTag {
		git(t, dir, "tag", "-f", "v1")
	}
	return "file://" + dir, shaA, shaB
}

func TestPinCloneOK(t *testing.T) {
	src, shaA, _ := pinRepo(t, false)
	dst := t.TempDir()
	if err := CloneToPinned(src, "v1", shaA, filepath.Join(dst, "plug")); err != nil {
		t.Fatalf("пин совпадает: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dst, "plug", "f.txt"))
	if err != nil || string(raw) != "A\n" {
		t.Fatalf("содержимое клона: %q err=%v (want A)", raw, err)
	}
}

func TestPinCatchMovedTag(t *testing.T) {
	// тег v1 переставили на B: клон HEAD=B != пин A → fail-closed
	src, shaA, shaB := pinRepo(t, true)
	dst := t.TempDir()
	err := CloneToPinned(src, "v1", shaA, filepath.Join(dst, "plug"))
	if err == nil {
		t.Fatalf("переставленный тег не пойман (HEAD=%s пин=%s)", shaB, shaA)
	}
}

func TestPinLegacyNoCommit(t *testing.T) {
	// commit="" — старое поведение (пин не задан)
	src, _, _ := pinRepo(t, true)
	dst := t.TempDir()
	if err := CloneToPinned(src, "v1", "", filepath.Join(dst, "plug")); err != nil {
		t.Fatalf("legacy (без пина): %v", err)
	}
}
