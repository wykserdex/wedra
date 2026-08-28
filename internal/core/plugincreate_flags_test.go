package core

// M5, тестер №2: флаги --author/--description у plugin create
// (с TODO в author/description скелет «выглядит как недоделка»).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCreateArgs_FlagsAfterPath(t *testing.T) {
	// ровно командная строка из фидбека тестера №2
	dir, o, err := ParseCreateArgs([]string{"plugins/url_checker", "--author", "me", "--description", "Checks URLs"})
	if err != nil {
		t.Fatal(err)
	}
	if dir != "plugins/url_checker" || o.Author != "me" || o.Description != "Checks URLs" {
		t.Fatalf("dir=%q opts=%+v", dir, o)
	}
}

func TestParseCreateArgs_EqFormAndFlagsFirst(t *testing.T) {
	dir, o, err := ParseCreateArgs([]string{"--author=me", "plugins/x", "--description=Does x"})
	if err != nil || dir != "plugins/x" || o.Author != "me" || o.Description != "Does x" {
		t.Fatalf("dir=%q opts=%+v err=%v", dir, o, err)
	}
	if _, _, err := ParseCreateArgs([]string{"plugins/x", "--bogus"}); err == nil ||
		!strings.Contains(err.Error(), "неизвестный флаг") {
		t.Fatalf("неизвестный флаг должен ругаться: %v", err)
	}
	if _, _, err := ParseCreateArgs([]string{"--author"}); err == nil ||
		!strings.Contains(err.Error(), "нужно значение") {
		t.Fatalf("флаг без значения: %v", err)
	}
	if _, _, err := ParseCreateArgs([]string{"a", "b"}); err == nil ||
		!strings.Contains(err.Error(), "лишний") {
		t.Fatalf("два пути: %v", err)
	}
}

func TestCreatePluginWith_FlagsFillManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins", "url_checker")
	if _, err := CreatePluginWith(dir, CreateOptions{Author: "me", Description: "Checks URLs"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `author: "me"`) || !strings.Contains(s, `description: "Checks URLs"`) {
		t.Fatalf("флаги не попали в манифест:\n%s", s)
	}
	if strings.Contains(s, `author: "TODO`) || strings.Contains(s, `description: "TODO`) {
		t.Fatalf("TODO не должны оставаться при заданных флагах:\n%s", s)
	}
	// скелет с флагами остаётся валидным — гарантия генератора не сломана
	if errs := ValidatePluginDir(dir); len(errs) > 0 {
		t.Fatalf("скелет с флагами невалиден: %v", errs)
	}
}

func TestCreatePlugin_DefaultsStillExplicit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bare_one")
	if _, err := CreatePlugin(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	// TODO без флагов остаются, но с подсказкой, КАК задать — обратная совместимость
	if !strings.Contains(string(raw), "--author me") {
		t.Fatalf("дефолтный TODO должен подсказывать флаг:\n%s", raw)
	}
}
