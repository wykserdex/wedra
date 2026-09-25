package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"wedra/internal/common"
)

// Registry v0.1 — формат заморозить как протокол.
// Файл registry.yaml лежит в корне репозитория-реестра.
//
// Плагин/пресет = запись (Entry): source (git-репо), path (подпуть),
// version (тег/ветка). Установленный плагин живёт в <pluginsDir>/<name>
// и несёт lock-файл .wedra с версией (для пиннинга из `name@version`).

type Entry struct {
	Source      string `yaml:"source"`      // git-URL (или локальный путь) репозитория
	Path        string `yaml:"path"`        // подпуть внутри репо (плагин — каталог, пресет — файл); дефолт "."
	Version     string `yaml:"version"`     // тег или ветка; дефолт "main"
	Commit      string `yaml:"commit"`      // v0.28a: полный SHA пина (теги переставляемы — SHA нет)
	Description string `yaml:"description"` // опционально
}

type Registry struct {
	Version string           `yaml:"version"` // версия формата реестра, "0.1"
	Plugins map[string]Entry `yaml:"plugins"`
	Presets map[string]Entry `yaml:"presets"`
}

const (
	FormatVersion      = "0.1"
	RegistryFile       = "registry.yaml"
	AddonsRegistryFile = "registry-addons.yaml"
	DefaultRegistryURL = "https://github.com/wykserdex/wedra"
)

// DefaultSource — локальный registry.yaml (или registry-addons.yaml) в CWD,
// иначе дефолтный URL-реестр.
func DefaultSource() string {
	if _, err := os.Stat(RegistryFile); err == nil {
		return RegistryFile
	}
	if _, err := os.Stat(AddonsRegistryFile); err == nil {
		return AddonsRegistryFile
	}
	return DefaultRegistryURL
}

// Handle — загруженный реестр.
type Handle struct {
	Registry *Registry
	Dir      string // каталог с registry.yaml (для локального — сам источник)
	tmp      string // временный клон (если источник был URL)
}

func (h *Handle) Close() error {
	if h.tmp != "" {
		return os.RemoveAll(h.tmp)
	}
	return nil
}

func (h *Handle) GetPlugin(name string) (Entry, bool) {
	e, ok := h.Registry.Plugins[name]
	return e, ok
}

func (h *Handle) GetPreset(name string) (Entry, bool) {
	e, ok := h.Registry.Presets[name]
	return e, ok
}

func (h *Handle) PluginNames() []string {
	out := make([]string, 0, len(h.Registry.Plugins))
	for n := range h.Registry.Plugins {
		out = append(out, n)
	}
	return out
}

func (h *Handle) PresetNames() []string {
	out := make([]string, 0, len(h.Registry.Presets))
	for n := range h.Registry.Presets {
		out = append(out, n)
	}
	return out
}

// Load — источник: локальный каталог с registry.yaml или git-URL (клон depth-1).
func Load(source string) (*Handle, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = DefaultSource()
	}

	// локальный каталог
	if fi, err := os.Stat(source); err == nil && fi.IsDir() {
		raw, err := os.ReadFile(filepath.Join(source, RegistryFile))
		if err != nil {
			return nil, fmt.Errorf("реестр %s: %w", source, err)
		}
		reg, err := parseRegistry(raw)
		if err != nil {
			return nil, err
		}
		return &Handle{Registry: reg, Dir: source}, nil
	}

	// локальный файл (registry.yaml в CWD или относительный путь)
	if fi, err := os.Stat(source); err == nil && fi.Mode().IsRegular() {
		raw, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		reg, err := parseRegistry(raw)
		if err != nil {
			return nil, err
		}
		return &Handle{Registry: reg, Dir: filepath.Dir(source)}, nil
	}

	// git-URL (или путь к bare-репо)
	tmp, err := os.MkdirTemp("", "wedra-registry-*")
	if err != nil {
		return nil, err
	}
	ref := source
	if strings.HasSuffix(source, "/") {
		ref = strings.TrimSuffix(source, "/")
	}
	cmd := exec.Command("git", "clone", "--depth", "1", "--quiet", ref, tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("клон реестра %s: %s: %s", source, err, string(out))
	}
	raw, err := os.ReadFile(filepath.Join(tmp, RegistryFile))
	if err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("в реестре %s нет %s", source, RegistryFile)
	}
	reg, err := parseRegistry(raw)
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &Handle{Registry: reg, Dir: tmp, tmp: tmp}, nil
}

func parseRegistry(raw []byte) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("registry.yaml: некорректный YAML: %w", err)
	}
	reg.Plugins = normalizeEntries(reg.Plugins)
	reg.Presets = normalizeEntries(reg.Presets)
	for name, entry := range reg.Plugins {
		if err := ValidateComponent(name); err != nil {
			return nil, fmt.Errorf("registry.yaml: плагин: %w", err)
		}
		if !safeEntryPath(entry.Path) {
			return nil, fmt.Errorf("registry.yaml: плагин %q: небезопасный path %q", name, entry.Path)
		}
		if err := ValidateCommit(entry.Commit); err != nil {
			return nil, fmt.Errorf("registry.yaml: плагин %q: %w", name, err)
		}
	}
	for name, entry := range reg.Presets {
		if err := ValidateComponent(name); err != nil {
			return nil, fmt.Errorf("registry.yaml: пресет: %w", err)
		}
		if !safeEntryPath(entry.Path) {
			return nil, fmt.Errorf("registry.yaml: пресет %q: небезопасный path %q", name, entry.Path)
		}
		if err := ValidateCommit(entry.Commit); err != nil {
			return nil, fmt.Errorf("registry.yaml: пресет %q: %w", name, err)
		}
	}
	return &reg, nil
}

func safeEntryPath(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") || (len(normalized) >= 2 && normalized[1] == ':') {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(normalized))
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func ValidateComponent(name string) error {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) || strings.ContainsRune(name, 0) || filepath.IsAbs(name) || filepath.Clean(name) != name {
		return fmt.Errorf("небезопасное имя компонента %q", name)
	}
	return nil
}

func normalizeEntries(in map[string]Entry) map[string]Entry {
	if in == nil {
		in = map[string]Entry{}
	}
	for name, e := range in {
		if e.Path == "" {
			e.Path = "."
		} else {
			e.Path = strings.ReplaceAll(e.Path, `\`, "/")
		}
		if e.Version == "" {
			e.Version = "main"
		}
		in[name] = e
	}
	return in
}

// IsLocalRef — локальный путь: содержит разделитель пути, начинается с "." или "/".
// Голое имя (и имя@версия) — это имя из реестра.
// v0.29: Windows-абсолюты и Windows-разделители тоже считаются локальными.
func IsLocalRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if common.IsBuiltinNamespace(ref) {
		return true
	}
	if strings.HasPrefix(ref, ".") || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) {
		return true
	}
	if filepath.IsAbs(ref) {
		return true
	}
	if len(ref) >= 2 && ref[1] == ':' {
		return true
	}
	return strings.ContainsAny(ref, `/\`)
}

// SplitRef — "name" или "name@version" → (name, version).
func SplitRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

func NormalizePluginRef(ref string) (name, version string, ok bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(ref), `\`, "/")
	for _, prefix := range []string{"plugins/official/", "plugins/community/"} {
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		rest := strings.TrimPrefix(normalized, prefix)
		if rest == "" || strings.Contains(rest, "/") {
			return "", "", false
		}
		name, version = SplitRef(rest)
		return name, version, true
	}
	return "", "", false
}

// RefToDir — резолвит ссылку `plugin:` в локальный каталог плагина.
// Локальные пути возвращаются как есть. Реестровые имена ищутся в
// <pluginsDir>/<name>, затем в <pluginsDir>/official/<name> и
// <pluginsDir>/community/<name>; flat-layout имеет приоритет.
// Запрошенная версия (@version) сверяется с lock-файлом .wedra.
func RefToDir(ref, pluginsDir string) (string, error) {
	if common.IsBuiltinNamespace(ref) {
		return "", fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	if IsLocalRef(ref) {
		return ref, nil
	}
	name, ver := SplitRef(ref)
	candidates := []string{
		filepath.Join(pluginsDir, name),
		filepath.Join(pluginsDir, "official", name),
		filepath.Join(pluginsDir, "community", name),
	}
	dir := ""
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "plugin.yaml")); err == nil {
			dir = candidate
			break
		}
	}
	if dir == "" {
		return "", fmt.Errorf("плагин %q не установлен: orchestrator plugin install %s", ref, ref)
	}
	if ver != "" {
		installed, ok := InstalledVersion(dir)
		if !ok {
			return "", fmt.Errorf("плагин %q: нет lock-файла .wedra, версия неизвестна — перустанови: orchestrator plugin install %s", name, ref)
		}
		if installed != ver {
			return "", fmt.Errorf("плагин %q: установлена версия %s, пайплайн требует %s — orchestrator plugin install %s", name, installed, ver, ref)
		}
	}
	return dir, nil
}
