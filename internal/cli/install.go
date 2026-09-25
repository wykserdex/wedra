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
	if err := registry.ValidateComponent(name); err != nil {
		return err
	}
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
	if err := registry.ValidateEntry(entry, true); err != nil {
		return err
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
	if err := installPluginDir(srcDir, destDir, name, entry, version); err != nil {
		return err
	}
	fmt.Printf("  + %s (%s) → %s\n", name, version, destDir)
	return nil
}

func installPluginDir(srcDir, destDir, name string, entry registry.Entry, version string) error {
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(destDir)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := registry.CopyDir(srcDir, staging); err != nil {
		return err
	}
	lock := registry.Lock{Name: name, Source: entry.Source, Path: entry.Path, Version: version}
	if err := registry.WriteLock(staging, lock); err != nil {
		return err
	}
	if errs := core.ValidatePluginDir(staging); len(errs) > 0 {
		for _, e := range errs {
			fmt.Println("  ✗ манифест:", e)
		}
		return fmt.Errorf("плагин %s не прошёл проверку манифеста", name)
	}

	backup := ""
	if _, err := os.Stat(destDir); err == nil {
		backup, err = os.MkdirTemp(parent, "."+filepath.Base(destDir)+".old-*")
		if err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
		if err := os.Rename(destDir, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staging, destDir); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, destDir); restoreErr != nil {
				return fmt.Errorf("replace %s: %w; restore: %v", destDir, err, restoreErr)
			}
		}
		return err
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
	}
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
		if entry.Commit != "" {
			if err := registry.VerifyCheckoutCommit(localSource, entry.Commit); err != nil {
				return "", "", fmt.Errorf("локальный source не соответствует pin: %w", err)
			}
		}
		return p, "", nil
	}
	if fi, e := os.Stat(entry.Source); e == nil && fi.IsDir() {
		if entry.Commit != "" {
			if err := registry.VerifyCheckoutCommit(entry.Source, entry.Commit); err != nil {
				return "", "", fmt.Errorf("локальный source не соответствует pin: %w", err)
			}
		}
		return filepath.Join(entry.Source, entry.Path), "", nil
	}
	if localRegistryDir != "" && entry.Commit == "" {
		// плагин — каталог, пресет — файл
		cand := filepath.Join(localRegistryDir, entry.Path)
		if _, e := os.Stat(cand); e != nil {
			cand = filepath.Join(localRegistryDir, "plugins", filepath.Base(filepath.FromSlash(entry.Path)))
		}
		if fi, e := os.Stat(cand); e == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			return cand, "", nil
		}
	}
	tmp, err = os.MkdirTemp("", "wedra-plugin-*")
	if err != nil {
		return "", "", err
	}
	if err := registry.CloneToPinned(entry.Source, version, entry.Commit, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", "", err
	}
	return filepath.Join(tmp, entry.Path), tmp, nil
}

// ── pipeline install ───────────────────────────────────────────────────────

type pipelineInstallResult struct {
	PresetName string
	OutFile    string
	Installed  int
	Present    int
	Warnings   []string
}

func commitPresetFile(staged, target string) error {
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return os.Rename(staged, target)
		}
		return err
	}
	backup, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".old-*")
	if err != nil {
		return err
	}
	backupName := backup.Name()
	if err := backup.Close(); err != nil {
		os.Remove(backupName)
		return err
	}
	if err := os.Remove(backupName); err != nil {
		return err
	}
	if err := os.Rename(target, backupName); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		if restoreErr := os.Rename(backupName, target); restoreErr != nil {
			return fmt.Errorf("replace %s: %w; restore: %v", target, err, restoreErr)
		}
		return err
	}
	return os.Remove(backupName)
}

func installPipelinePreset(raw []byte, fallbackName, registrySrc string) (pipelineInstallResult, error) {
	pf, err := pipeline.LoadPipelineFileFromBytes(raw)
	if err != nil {
		return pipelineInstallResult{}, fmt.Errorf("пресет не распарсился как пайплайн: %w", err)
	}
	for i := range pf.Pipeline.Steps {
		if name, version, ok := registry.NormalizePluginRef(pf.Pipeline.Steps[i].Plugin); ok {
			if version != "" {
				pf.Pipeline.Steps[i].Plugin = name + "@" + version
			} else {
				pf.Pipeline.Steps[i].Plugin = name
			}
		}
	}
	normalized, err := yaml.Marshal(pf)
	if err != nil {
		return pipelineInstallResult{}, fmt.Errorf("не удалось нормализовать пресет: %w", err)
	}
	pname := pf.Pipeline.Name
	if pname == "" {
		pname = fallbackName
	}
	if err := registry.ValidateComponent(pname); err != nil {
		return pipelineInstallResult{}, fmt.Errorf("небезопасное имя пресета: %w", err)
	}
	if err := os.MkdirAll("examples", 0o755); err != nil {
		return pipelineInstallResult{}, err
	}
	outFile := filepath.Join("examples", pname+".yaml")
	staged, err := os.CreateTemp("examples", "."+pname+".yaml.tmp-*")
	if err != nil {
		return pipelineInstallResult{}, err
	}
	stagedName := staged.Name()
	committed := false
	defer func() {
		if !committed {
			os.Remove(stagedName)
		}
	}()
	if _, err := staged.Write(normalized); err != nil {
		staged.Close()
		return pipelineInstallResult{}, err
	}
	if err := staged.Close(); err != nil {
		return pipelineInstallResult{}, err
	}
	if err := os.Chmod(stagedName, 0o644); err != nil {
		return pipelineInstallResult{}, err
	}

	wantVer := map[string]string{}
	for _, st := range pf.Pipeline.Steps {
		if registry.IsLocalRef(st.Plugin) {
			continue
		}
		nm, vr := registry.SplitRef(st.Plugin)
		if prev, ok := wantVer[nm]; ok && prev != "" && vr != "" && prev != vr {
			return pipelineInstallResult{}, fmt.Errorf("плагин %s требует разные версии: %s и %s", nm, prev, vr)
		}
		if prev, ok := wantVer[nm]; !ok || prev == "" {
			wantVer[nm] = vr
		}
	}
	result := pipelineInstallResult{PresetName: pname, OutFile: outFile}
	for nm, vr := range wantVer {
		installedDir, err := registry.RefToDir(nm, "plugins")
		need := err != nil
		if !need && vr != "" {
			if iv, ok := registry.InstalledVersion(installedDir); !ok || iv != vr {
				need = true
			}
		}
		if need {
			if err := doPluginInstall(nm, vr, registrySrc, "plugins"); err != nil {
				return result, fmt.Errorf("автоустановка %s: %w", nm, err)
			}
			result.Installed++
		} else {
			result.Present++
		}
	}

	errs, warns := core.Validate(pf, core.NewEngine())
	result.Warnings = warns
	if len(errs) > 0 {
		return result, fmt.Errorf("валидация пресета: %s", strings.Join(errs, "; "))
	}
	if err := commitPresetFile(stagedName, outFile); err != nil {
		return result, err
	}
	committed = true
	return result, nil
}

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
	result, err := installPipelinePreset(raw, name, registrySrc)
	if err != nil {
		fmt.Println("ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("▶ пресет %q → %s\n", result.PresetName, result.OutFile)
	fmt.Printf("  плагины: %d установлено, %d уже на месте\n", result.Installed, result.Present)
	for _, warning := range result.Warnings {
		fmt.Println("  · предупреждение:", warning)
	}
	fmt.Printf("■ пресет %q готов: %s --yes\n", result.PresetName, result.OutFile)
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
	if err := registry.ValidateEntry(entry, true); err != nil {
		return nil, "", err
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
