package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"wedra/internal/core"
	"wedra/internal/pipeline"
	"wedra/internal/registry"
)

// ── plugin install ─────────────────────────────────────────────────────────

func RunPluginInstall(args []string) {
	ref, registrySrc, dest := "", "", "plugins"
	for _, a := range args {
		if strings.HasPrefix(a, "--registry=") {
			registrySrc = a[len("--registry="):]
		} else if strings.HasPrefix(a, "--dest=") {
			dest = a[len("--dest="):]
		} else if !strings.HasPrefix(a, "-") {
			ref = a
		}
	}
	if ref == "" {
		fmt.Println("нужно имя плагина: wedra plugin install <name>[@version] [--registry=<url|path>] [--dest=plugins]")
		os.Exit(2)
	}
	name, ver := registry.SplitRef(ref)
	if err := doPluginInstall(name, ver, registrySrc, dest); err != nil {
		fmt.Println("установка не удалась:", err)
		os.Exit(1)
	}
}

// doPluginInstall — ядро установки, общее для `plugin install`
// и автоустановки из `pipeline install`.
func doPluginInstall(name, ver, registrySrc, dest string) error {
	h, err := registry.Load(registrySrc)
	if err != nil {
		return err
	}
	defer h.Close()

	entry, ok := h.GetPlugin(name)
	if !ok {
		names := h.PluginNames()
		sort.Strings(names)
		return fmt.Errorf("плагин %q нет в реестре (доступно: %s)", name, strings.Join(names, ", "))
	}
	version := ver
	if version == "" {
		version = entry.Version
	}

	srcDir, tmp, err := pluginSourceDir(entry, h.Dir, version, "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if _, err := os.Stat(filepath.Join(srcDir, "plugin.yaml")); err != nil {
		return fmt.Errorf("в %s нет plugin.yaml (source=%s path=%s version=%s)", srcDir, entry.Source, entry.Path, version)
	}

	destDir := filepath.Join(dest, name)
	if _, err := os.Stat(destDir); err == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return err
		}
	}
	if err := registry.CopyDir(srcDir, destDir); err != nil {
		return err
	}
	lock := registry.Lock{Name: name, Source: entry.Source, Path: entry.Path, Version: version}
	if err := registry.WriteLock(destDir, lock); err != nil {
		return err
	}

	errs := core.ValidatePluginDir(destDir)
	for _, e := range errs {
		fmt.Println("  ✗ манифест:", e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("плагин %s установлен, но манифест некорректен", name)
	}
	fmt.Printf("  + %s (%s) → %s\n", name, version, destDir)
	return nil
}

// pluginSourceDir — где лежат файлы плагина:
// 1) source — локальный каталог (оффлайн-реестр)
// 2) реестр загружен из локального каталога и path существует рядом (оффлайн-шорткат)
// 3) git clone source@version (сеть)
func pluginSourceDir(entry registry.Entry, localRegistryDir, version, localSource string) (srcDir, tmp string, err error) {
	// v0.17: явное override — source совпадает с этим локальным чеккаутом.
	// v0.21: путь обязан резолвиться локально — trust-гейт не молчит и не
	// уходит в клон, если файл в локальном source отсутствует.
	if localSource != "" && sameRepo(entry.Source, localSource) {
		p := filepath.Join(localSource, entry.Path)
		if _, e := os.Stat(p); e != nil {
			return "", "", fmt.Errorf("запись %s: путь %s не найден в локальном source (--local-source=%s)", entry.Path, entry.Path, localSource)
		}
		return p, "", nil
	}
	if fi, e := os.Stat(entry.Source); e == nil && fi.IsDir() {
		return filepath.Join(entry.Source, entry.Path), "", nil
	}
	if localRegistryDir != "" {
		// плагин — каталог, пресет — файл
		cand := filepath.Join(localRegistryDir, entry.Path)
		if fi, e := os.Stat(cand); e == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			return cand, "", nil
		}
	}
	tmp, err = os.MkdirTemp("", "wedra-plugin-*")
	if err != nil {
		return "", "", err
	}
	if err := registry.CloneTo(entry.Source, version, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", "", err
	}
	return filepath.Join(tmp, entry.Path), tmp, nil
}

// ── pipeline install ───────────────────────────────────────────────────────

func RunPipelineInstall(args []string) {
	preset, registrySrc := "", ""
	for _, a := range args {
		if strings.HasPrefix(a, "--registry=") {
			registrySrc = a[len("--registry="):]
		} else if !strings.HasPrefix(a, "-") {
			preset = a
		}
	}
	if preset == "" {
		fmt.Println("нужен пресет: wedra pipeline install <name|file.yaml|url> [--registry=<url|path>]")
		os.Exit(2)
	}

	raw, name, err := fetchPreset(preset, registrySrc, "")
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}

	var pf pipeline.PipelineFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		fmt.Println("пресет не распарсился как пайплайн:", err)
		os.Exit(1)
	}
	pname := pf.Pipeline.Name
	if pname == "" {
		pname = name
	}
	outFile := filepath.Join("examples", pname+".yaml")
	if err := os.MkdirAll("examples", 0o755); err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outFile, raw, 0o644); err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("▶ пресет %q → %s\n", pname, outFile)

	// автоустановка недостающих плагинов (реестровые ссылки)
	// имя → требуемая версия ("" = любая); конфликты версий одного плагина — ошибка
	wantVer := map[string]string{}
	for _, st := range pf.Pipeline.Steps {
		if registry.IsLocalRef(st.Plugin) {
			continue
		}
		nm, vr := registry.SplitRef(st.Plugin)
		if prev, ok := wantVer[nm]; ok && prev != "" && vr != "" && prev != vr {
			fmt.Printf("ошибка: плагин %s в одном пайплайне требует разные версии: %s и %s\n", nm, prev, vr)
			os.Exit(1)
		}
		if vr != "" {
			wantVer[nm] = vr
		}
	}
	installed, present := 0, 0
	for nm, vr := range wantVer {
		dir := filepath.Join("plugins", nm)
		need := false
		if _, err := registry.RefToDir(nm, "plugins"); err != nil {
			need = true
		} else if vr != "" {
			if iv, ok := registry.InstalledVersion(dir); ok && iv != vr {
				need = true // версия не совпадает — переустановить под пин
			}
		}
		if need {
			if err := doPluginInstall(nm, vr, registrySrc, "plugins"); err != nil {
				fmt.Println("ошибка автоустановки", nm, ":", err)
				os.Exit(1)
			}
			installed++
		} else {
			present++
		}
	}
	fmt.Printf("  плагины: %d установлено, %d уже на месте\n", installed, present)

	// финальная проверка совместимости
	eng := core.NewEngine()
	errs, warns := core.Validate(&pf, eng)
	for _, w := range warns {
		fmt.Println("  · предупреждение:", w)
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println("  ✗", e)
		}
		fmt.Println("пресет установлен, но валидация не прошла")
		os.Exit(1)
	}
	fmt.Printf("■ пресет %q готов: %s --yes\n", pname, outFile)
}

// fetchPreset — имя из реестра, локальный .yaml или http(s) URL.
func fetchPreset(preset, registrySrc, localSource string) ([]byte, string, error) {
	// 1) локальный файл
	if strings.HasSuffix(preset, ".yaml") || strings.HasSuffix(preset, ".yml") {
		if _, e := os.Stat(preset); e == nil {
			raw, e2 := os.ReadFile(preset)
			name := strings.TrimSuffix(filepath.Base(preset), filepath.Ext(preset))
			return raw, name, e2
		}
	}
	// 2) URL
	if strings.HasPrefix(preset, "http://") || strings.HasPrefix(preset, "https://") {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, e2 := client.Get(preset)
		if e2 != nil {
			return nil, "", fmt.Errorf("загрузка %s: %w", preset, e2)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, "", fmt.Errorf("%s: HTTP %d", preset, resp.StatusCode)
		}
		raw, e2 := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if e2 != nil {
			return nil, "", e2
		}
		name := strings.TrimSuffix(filepath.Base(preset), filepath.Ext(preset))
		return raw, name, nil
	}
	// 3) реестр
	h, e2 := registry.Load(registrySrc)
	if e2 != nil {
		return nil, "", e2
	}
	defer h.Close()
	entry, ok := h.GetPreset(preset)
	if !ok {
		names := h.PresetNames()
		sort.Strings(names)
		return nil, "", fmt.Errorf("пресет %q нет в реестре (доступно: %s)", preset, strings.Join(names, ", "))
	}
	// для пресета src — путь к самому файлу
	src, tmp, e2 := pluginSourceDir(entry, h.Dir, entry.Version, localSource)
	if e2 != nil {
		return nil, "", e2
	}
	defer os.RemoveAll(tmp)
	raw, e2 := os.ReadFile(src)
	if e2 != nil {
		return nil, "", fmt.Errorf("пресет %s: %w", preset, e2)
	}
	return raw, preset, nil
}
