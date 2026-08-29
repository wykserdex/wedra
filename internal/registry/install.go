package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Lock — метка установленного плагина (.wedra в каталоге плагина).
// Нужен для пиннинга: пайплайн может требовать name@version, раннер сверяет.
type Lock struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"`
}

const LockFile = ".wedra"

func WriteLock(dir string, l Lock) error {
	l.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(&l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, LockFile), raw, 0o644)
}

func ReadLock(dir string) (Lock, error) {
	var l Lock
	raw, err := os.ReadFile(filepath.Join(dir, LockFile))
	if err != nil {
		return l, err
	}
	err = json.Unmarshal(raw, &l)
	return l, err
}

func InstalledVersion(dir string) (string, bool) {
	l, err := ReadLock(dir)
	if err != nil || l.Version == "" {
		return "", false
	}
	return l.Version, true
}

// CloneTo — git clone depth-1 source@ref → dir (dir создаётся).
func CloneTo(source, ref, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", ref, "--quiet", source, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %s@%s: %s: %s", source, ref, err, string(out))
	}
	return nil
}

// CopyDir — рекурсивное копирование (без скрытых файлов исходника,
// кроме .gitignore ничего не тащим; .wedra из клона не тащим — свой пишем).
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == ".wedra" {
				return filepath.SkipDir
			}
			if path == src {
				return os.MkdirAll(dst, 0o755)
			}
			return os.MkdirAll(filepath.Join(dst, stringsRel(src, path)), 0o755)
		}
		base := filepath.Base(path)
		if base == ".wedra" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, stringsRel(src, path)), data, 0o644)
	})
}

func stringsRel(base, p string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return filepath.Base(p)
	}
	return rel
}
