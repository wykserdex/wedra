package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"wedra/internal/core"
)

func RunPluginValidate(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен путь к плагину: wedra plugin validate <dir>")
		os.Exit(2)
	}
	errs := core.ValidatePluginDir(args[0])
	for _, e := range errs {
		fmt.Println("  ✗", e)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
	fmt.Println("OK: манифест плагина корректен")
}

func RunPluginTest(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен путь к плагину: wedra plugin test <dir>")
		os.Exit(2)
	}
	dir := args[0]
	spec := ""
	for i := 1; i+1 < len(args); i++ {
		if args[i] == "--spec" {
			spec = args[i+1]
		}
	}
	fmt.Printf("▶ plugin test %s\n", dir)
	passed, failed, err := core.RunPluginTests(dir, spec, false)
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("■ plugin test: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func RunPluginCreate(args []string) {
	dir, opts, err := core.ParseCreateArgs(args)
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(2)
	}
	id, err := core.CreatePluginWith(dir, opts)
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("▶ создан плагин %s → %s\n", id, dir)
	fmt.Println("  plugin.yaml + main.py + plugin.test.yaml + README.md")

	if errs := core.ValidatePluginDir(dir); len(errs) > 0 {
		fmt.Println("внутренняя ошибка генератора — скелет невалиден:")
		for _, e := range errs {
			fmt.Println("  ✗", e)
		}
		os.Exit(1)
	}
	fmt.Println("  ✓ манифест валиден")

	passed, failed, err := core.RunPluginTests(dir, "", false)
	if err != nil {
		fmt.Printf("  · тесты не запустились (%v) — проверьте python3\n", err)
	} else if failed > 0 {
		fmt.Println("внутренняя ошибка генератора — стартовые тесты красные")
		os.Exit(1)
	} else {
		fmt.Printf("  ✓ %d стартовых теста зелёные\n", passed)
	}
	fmt.Printf("\nДальше: правьте %s/main.py, затем wedra plugin test %s\n", dir, dir)
}

func RunPluginInspect(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен путь к плагину: wedra plugin inspect <dir>")
		os.Exit(2)
	}
	eng := core.NewEngine()
	m, err := eng.LoadManifest(args[0])
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("Plugin: %s\n", m.ID)
	fmt.Printf("Version: %s (platform_api %s)\n", m.Version, m.PlatformAPI)
	fmt.Printf("Description: %s\n", m.Description)
	fmt.Printf("Author: %s\n", m.Author)
	fmt.Printf("Runtime: %s %s\n", m.Runtime.Type, m.Runtime.Entry)
	fmt.Println("Input:")
	for k, p := range m.Input {
		fmt.Printf("  %s: type=%s format=%s optional=%v from=%s\n", k, p.Type, p.Format, p.Optional, p.From)
	}
	fmt.Println("Output:")
	for k, p := range m.Output {
		fmt.Printf("  %s: type=%s format=%s optional=%v\n", k, p.Type, p.Format, p.Optional)
	}
	fmt.Printf("Permissions: %+v\n", m.Permissions)
}

func RunPluginList(args []string) {
	plugins := core.ScanPlugins()
	if len(plugins) == 0 {
		fmt.Println("плагины не найдены в plugins/official и plugins/community")
		return
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	fmt.Printf("Найдено %d плагинов:\n", len(plugins))
	for _, p := range plugins {
		fmt.Printf("  %-24s %-10s %s (%s)\n", p.ID, p.Version, p.Dir, p.Description)
	}
}

func RunPluginSearch(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен запрос: wedra plugin search <query>")
		os.Exit(2)
	}
	query := strings.ToLower(strings.Join(args, " "))
	plugins := core.ScanPlugins()
	matched := []core.Manifest{}
	for _, p := range plugins {
		hay := strings.ToLower(p.ID + " " + p.Description + " " + p.Author)
		if strings.Contains(hay, query) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		fmt.Printf("по запросу %q ничего не найдено (всего %d плагинов)\n", query, len(plugins))
		return
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	fmt.Printf("По запросу %q найдено %d:\n", query, len(matched))
	for _, p := range matched {
		fmt.Printf("  %-24s %-10s %s\n    %s\n", p.ID, p.Version, p.Dir, p.Description)
	}
}
