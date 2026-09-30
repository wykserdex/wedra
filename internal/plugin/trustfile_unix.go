//go:build !windows

package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// checkTrustConfigFile — конфиг доверия решает, чей код получит права
// пользователя, поэтому его целостность проверяется до чтения (аудит N3).
//
// Файл по умолчанию лежит в рабочем каталоге (рядом с registry.yaml), то есть
// там, куда может писать любой процесс с доступом к этому каталогу. Проверка не
// заменяет изоляцию, но закрывает дешёвые пути: файл, доступный на запись
// группе или всем, файл чужого пользователя, симлинк (его можно перенаправить
// после проверки) и каталог, в котором любой может подменить файл.
//
// Любое нарушение — ошибка, а не предупреждение: молча расширенный allow-list
// хуже, чем остановленный запуск.
func checkTrustConfigFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("это симлинк — конфиг доверия должен быть обычным файлом")
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("это не обычный файл")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("файл доступен на запись группе или всем (%#o) — выполните chmod go-w", fi.Mode().Perm())
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if uid := uint32(os.Getuid()); st.Uid != uid && st.Uid != 0 {
			return fmt.Errorf("файл принадлежит другому пользователю (uid %d, а запуск от uid %d)", st.Uid, uid)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pi, err := os.Stat(filepath.Dir(abs)); err == nil {
		if pi.Mode().Perm()&0o002 != 0 && pi.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("каталог %s доступен на запись всем без sticky-бита — файл можно подменить", filepath.Dir(abs))
		}
	}
	return nil
}
