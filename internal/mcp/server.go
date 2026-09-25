package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"wedra/internal/common"
	"wedra/internal/core"
	"wedra/internal/execution"
	"wedra/internal/gate"
	"wedra/internal/journal"
	"wedra/internal/pipeline"
	"wedra/internal/registry"
)

// Version — версия сервера в initialize (cli проставляет из VERSION).
var Version = "dev"

const (
	defaultProtocolVersion = "2024-11-05"
	maxWaitSeconds         = 300
	maxRetainedRuns        = 128
)

var supportedProtocolVersions = []string{defaultProtocolVersion}

// Server — MCP-сервер: JSON-RPC stdio + 7 инструментов поверх core/pipeline/execution.
// Решения гейтов через MCP невозможны никогда; get_run в waiting_human
// просит пользователя одобрить в окне wedra.
type Server struct {
	pluginsDirs []string
	workDir     string
	runsDir     string
	engine      *core.Engine
	multi       *multiEngine

	human HumanChannel

	mu           sync.Mutex
	running      bool
	currentRunID string
	runs         map[string]*runState
	cancels      map[string]context.CancelFunc
}

type runState struct {
	id      string
	dir     string
	done    chan struct{}
	status  string
	errMsg  string
	code    string
	okItems int
	aborted int
}

// Options — флаги wedra mcp --plugins ... --workdir ...
type Options struct {
	PluginsDirs []string
	WorkDir     string
	RunsDir     string
	// Agent-track human channel for gates and cancellation.
	// nil — гейты MCP-ранов одобрить некому: run_pipeline с human_gate
	// отклоняется с E_NO_HUMAN_CHANNEL, а не висит в waiting_human вечно.
	Human HumanChannel
}

// HumanChannel — куда MCP-сервер отдаёт гейты ранов. Реализация (cli/mcp.go)
// регистрирует ChannelUI во встроенном HTTP-сервере с сессией и открывает
// браузер человеку. Агенту URL с ключом не выдаётся никогда.
type HumanChannel interface {
	AttachRun(runID string, ui *gate.ChannelUI, cancel context.CancelFunc)
	DetachRun(runID string)
	// GateWaiting — ран дошёл до гейта (первый раз): показать человеку.
	GateWaiting(runID string)
	// PublicURL — адрес консоли БЕЗ ключа (для подсказки агенту).
	PublicURL(runID string) string
}

func NewServer(opts Options) (*Server, error) {
	workDir := opts.WorkDir
	if workDir == "" {
		cwd, _ := os.Getwd()
		workDir = cwd
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}
	var absPlugins []string
	for _, d := range opts.PluginsDirs {
		if strings.TrimSpace(d) == "" {
			return nil, fmt.Errorf("плагины: каталог не указан")
		}
		if !filepath.IsAbs(d) {
			d = filepath.Join(absWork, d)
		}
		a, err := filepath.Abs(d)
		if err != nil {
			return nil, err
		}
		absPlugins = append(absPlugins, a)
	}
	if len(absPlugins) == 0 {
		absPlugins = []string{filepath.Join(absWork, "plugins")}
	}
	runsDir := opts.RunsDir
	if runsDir == "" {
		runsDir = filepath.Join(absWork, "var", "runs")
	} else if !filepath.IsAbs(runsDir) {
		runsDir = filepath.Join(absWork, runsDir)
	}
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return nil, err
	}
	eng := core.NewEngine()
	eng.PluginsDir = absPlugins[0]
	multi := &multiEngine{dirs: absPlugins, workDir: absWork}
	return &Server{
		pluginsDirs: absPlugins,
		workDir:     absWork,
		runsDir:     runsDir,
		engine:      eng,
		multi:       multi,
		runs:        map[string]*runState{},
		cancels:     map[string]context.CancelFunc{},
		human:       opts.Human,
	}, nil
}

// multiEngine — резолв плагинов по нескольким корням (песочница Фаза 4.4).
type multiEngine struct {
	dirs    []string
	workDir string
}

func parseManifestBytes(raw []byte) (*pipeline.Manifest, error) {
	var m pipeline.Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return nil, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("manifest: несколько YAML-документов")
		}
		return nil, err
	}
	return &m, nil
}

func (m *multiEngine) pluginRoots() []string {
	roots := append([]string{}, m.dirs...)
	if m.workDir != "" {
		roots = append(roots, m.workDir)
	}
	return roots
}

func (m *multiEngine) pluginPathAllowed(path string) bool {
	for _, root := range m.pluginRoots() {
		if pathWithinRoot(root, path) {
			return true
		}
	}
	return false
}

func (m *multiEngine) checkPluginDir(path string) error {
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: plugin %q содержит NUL", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: plugin %q не разрешается", path)
	}
	if !m.pluginPathAllowed(abs) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: plugin %q вне --plugins/--workdir", path)
	}
	resolved, err := resolvePathForContainment(abs)
	if err != nil || !m.pluginPathAllowed(resolved) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: plugin %q уходит через symlink/junction", path)
	}
	manifest := filepath.Join(abs, "plugin.yaml")
	if _, err := os.Lstat(manifest); err == nil && !pathWithinRoot(abs, manifest) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: plugin %q manifest уходит через symlink/junction", path)
	}
	return nil
}

func (m *multiEngine) checkPluginRef(ref string) error {
	if strings.Contains(ref, "..") {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: путь %q выходит за корни плагинов", ref)
	}
	if (strings.HasPrefix(ref, "/") || (filepath.Separator == '\\' && strings.HasPrefix(ref, `\`))) && !filepath.IsAbs(ref) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: %q вне --plugins/--workdir", ref)
	}
	if !registry.IsLocalRef(ref) {
		var boundaryErr error
		for _, root := range m.pluginRoots() {
			dir, err := registry.RefToDir(ref, root)
			if err != nil {
				continue
			}
			if err := m.checkPluginDir(dir); err != nil {
				boundaryErr = err
				continue
			}
			return nil
		}
		return boundaryErr
	}
	path := ref
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, `\`) {
		path = filepath.Join(m.workDir, path)
	}
	return m.checkPluginDir(path)
}

func (m *multiEngine) loadLocal(ref string) (*pipeline.Manifest, error) {
	path := ref
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, `\`) {
		path = filepath.Join(m.workDir, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := m.checkPluginDir(path); err != nil {
		return nil, err
	}
	eng := core.NewEngine()
	eng.PluginsDir = m.workDir
	return eng.LoadManifest(path)
}

func (m *multiEngine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	if pipeline.IsBuiltin(ref) {
		return &pipeline.Manifest{ID: "core/human_gate", Version: pipeline.PlatformAPI}, nil
	}
	if pipeline.IsBuiltinNamespace(ref) {
		return nil, fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	if registry.IsLocalRef(ref) {
		if err := m.checkPluginRef(ref); err != nil {
			return nil, err
		}
		return m.loadLocal(ref)
	}
	lastErr := fmt.Errorf("плагин %q не найден", ref)
	var boundaryErr error
	for _, dir := range m.dirs {
		candidate, err := registry.RefToDir(ref, dir)
		if err != nil {
			lastErr = err
			continue
		}
		if err := m.checkPluginDir(candidate); err != nil {
			boundaryErr = err
			continue
		}
		eng := core.NewEngine()
		eng.PluginsDir = dir
		if manifest, err := eng.LoadManifest(candidate); err == nil {
			return manifest, nil
		} else {
			lastErr = err
		}
	}
	candidate, err := registry.RefToDir(ref, m.workDir)
	if err == nil {
		if err := m.checkPluginDir(candidate); err != nil {
			boundaryErr = err
		} else {
			eng := core.NewEngine()
			eng.PluginsDir = m.workDir
			if manifest, err := eng.LoadManifest(candidate); err == nil {
				return manifest, nil
			} else {
				lastErr = err
			}
		}
	}
	if boundaryErr != nil {
		return nil, boundaryErr
	}
	return nil, lastErr
}

// checkPluginRef — песочница: абсолютные пути и .. за пределами корней запрещены.
func (s *Server) checkPluginRef(ref string) error {
	if pipeline.IsBuiltin(ref) {
		return nil
	}
	if pipeline.IsBuiltinNamespace(ref) {
		return fmt.Errorf("E_PLUGIN_LOAD: неизвестный встроенный модуль: %s", ref)
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: путь %q выходит за корни плагинов", ref)
	}
	if s.multi != nil {
		return s.multi.checkPluginRef(ref)
	}
	isAbs := filepath.IsAbs(ref) || strings.HasPrefix(ref, "/")
	if isAbs {
		abs := ref
		if !filepath.IsAbs(abs) {
			return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: %q вне --plugins/--workdir", ref)
		}
		abs, _ = filepath.Abs(abs)
		for _, root := range append(append([]string{}, s.pluginsDirs...), s.workDir) {
			if pathWithinRoot(root, abs) {
				return nil
			}
		}
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: %q вне --plugins/--workdir", ref)
	}
	return nil
}

func (s *Server) checkPathInWorkdir(p string) error {
	if p == "" {
		return nil
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: path %q содержит NUL", p)
	}
	candidate := p
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(s.workDir, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil || !pathWithinRoot(s.workDir, abs) {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: path %q вне --workdir", p)
	}
	return nil
}

func resolvePathForContainment(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current = filepath.Clean(current)
	for links := 0; links < 255; links++ {
		probe := current
		suffix := []string{}
		for {
			if target, linkErr := os.Readlink(probe); linkErr == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(probe), target)
				}
				current = filepath.Join(append([]string{target}, suffix...)...)
				break
			}
			if info, statErr := os.Lstat(probe); statErr == nil {
				if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
					return "", fmt.Errorf("reparse point: %s", probe)
				}
			} else if !os.IsNotExist(statErr) {
				return "", statErr
			}
			if resolved, evalErr := filepath.EvalSymlinks(probe); evalErr == nil {
				for _, part := range suffix {
					resolved = filepath.Join(resolved, part)
				}
				return filepath.Clean(resolved), nil
			}
			parent := filepath.Dir(probe)
			if parent == probe {
				return "", fmt.Errorf("path cannot be resolved: %s", current)
			}
			suffix = append([]string{filepath.Base(probe)}, suffix...)
			probe = parent
		}
	}
	return "", fmt.Errorf("too many links: %s", current)
}

func pathWithinRoot(root, path string) bool {
	rootAbs, err := resolvePathForContainment(root)
	if err != nil {
		return false
	}
	pathAbs, err := resolvePathForContainment(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func foreachInputKey(foreach, item string) string {
	if foreach == "" {
		return ""
	}
	if item == "" {
		return "item"
	}
	return item
}

func fileRefSourceDynamic(pf *pipeline.PipelineFile, st *pipeline.Step, source string) bool {
	if !strings.HasPrefix(source, "input.") {
		return true
	}
	key := strings.TrimPrefix(source, "input.")
	if key == "" || strings.Contains(key, ".") {
		return true
	}
	if pf.Pipeline.Foreach != "" && key == foreachInputKey(pf.Pipeline.Foreach, pf.Pipeline.ForeachItem) {
		return true
	}
	if st != nil && st.Foreach != "" && key == foreachInputKey(st.Foreach, st.ForeachItem) {
		return true
	}
	for i := range pf.Pipeline.Steps {
		step := &pf.Pipeline.Steps[i]
		if step.Foreach != "" && key == foreachInputKey(step.Foreach, step.ForeachItem) {
			return true
		}
	}
	return false
}

func (s *Server) checkPipelineSafety(pf *pipeline.PipelineFile) error {
	for i := range pf.Pipeline.Steps {
		st := &pf.Pipeline.Steps[i]
		if pipeline.IsBuiltin(st.Plugin) {
			continue
		}
		m, err := s.multi.LoadManifest(st.Plugin)
		if err != nil {
			if strings.HasPrefix(err.Error(), "E_PLUGIN_OUTSIDE_ROOT") {
				return err
			}
			continue
		}
		for _, permission := range m.Permissions.Network {
			if permission.AnyHost {
				return fmt.Errorf("E_NETWORK_DENIED: плагин %s заявил any_host в MCP", m.ID)
			}
			host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(permission.Host), "."))
			if host == "" {
				return fmt.Errorf("E_NETWORK_DENIED: пустой host в permissions плагина %s", m.ID)
			}
			if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
				return fmt.Errorf("E_NETWORK_DENIED: host %q запрещён в MCP", permission.Host)
			}
			if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()) {
				return fmt.Errorf("E_NETWORK_DENIED: private/loopback host %q запрещён в MCP", permission.Host)
			}
		}
		for name, port := range m.Input {
			if port.Format != "file_ref" {
				continue
			}
			source := pipeline.PortSource(name, port, st)
			if fileRefSourceDynamic(pf, st, source) {
				return fmt.Errorf("E_FILE_REF_UNCHECKED: плагин %s, порт %s, источник %q", m.ID, name, source)
			}
			key := strings.TrimPrefix(source, "input.")
			raw, ok := pf.Pipeline.Input[key]
			if !ok {
				continue
			}
			value, ok := raw.(string)
			if !ok {
				return fmt.Errorf("E_FILE_REF_UNCHECKED: input.%s должен быть строкой", key)
			}
			if err := s.checkPathInWorkdir(value); err != nil {
				return fmt.Errorf("E_FILE_REF_OUTSIDE_ROOT: input.%s: %w", key, err)
			}
		}
	}
	return nil
}

// loadPipeline — yaml или path (ровно один), с проверкой песочницы.
func (s *Server) loadPipeline(args map[string]interface{}) (*pipeline.PipelineFile, error) {
	yamlStr, _ := args["yaml"].(string)
	pathStr, _ := args["path"].(string)
	if yamlStr != "" && pathStr != "" {
		return nil, fmt.Errorf("укажите только один: yaml или path")
	}
	var raw []byte
	if pathStr != "" {
		if err := s.checkPathInWorkdir(pathStr); err != nil {
			return nil, err
		}
		full := pathStr
		if !filepath.IsAbs(full) {
			full = filepath.Join(s.workDir, pathStr)
		}
		var err error
		raw, err = os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", pathStr, err)
		}
	} else if yamlStr != "" {
		raw = []byte(yamlStr)
	} else {
		return nil, fmt.Errorf("нужен yaml или path")
	}
	pf, err := pipeline.LoadPipelineFileFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	for _, st := range pf.Pipeline.Steps {
		if err := s.checkPluginRef(st.Plugin); err != nil {
			return nil, err
		}
	}
	if err := s.checkPipelineSafety(pf); err != nil {
		return nil, err
	}
	return pf, nil
}

// validate — общий путь для validate/run/plan: загрузка + ValidateIssues.
func (s *Server) validateArgs(args map[string]interface{}) (*pipeline.PipelineFile, []pipeline.Issue, error) {
	pf, err := s.loadPipeline(args)
	if err != nil {
		return nil, nil, err
	}
	issues := pipeline.ValidateIssues(pf, s.multi)
	return pf, issues, nil
}

// Serve — главный цикл stdio.
func (s *Server) Serve(t *Transport) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	for {
		req, err := t.Read()
		if err != nil {
			wg.Wait()
			select {
			case writeErr := <-errCh:
				return writeErr
			default:
				return err
			}
		}
		if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		wg.Add(1)
		go func(req *Request) {
			defer wg.Done()
			res := s.handle(req)
			res.ID = req.ID
			if err := t.Write(res); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}(req)
	}
}

func negotiateProtocol(params json.RawMessage) (string, error) {
	if len(params) == 0 || string(params) == "null" {
		return defaultProtocolVersion, nil
	}
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.ProtocolVersion == "" {
		return defaultProtocolVersion, nil
	}
	for _, version := range supportedProtocolVersions {
		if p.ProtocolVersion == version {
			return version, nil
		}
	}
	return supportedProtocolVersions[0], nil
}

func (s *Server) handle(req *Request) *Response {
	switch req.Method {
	case "initialize":
		version, err := negotiateProtocol(req.Params)
		if err != nil {
			return &Response{Error: &RPCError{Code: -32602, Message: "invalid initialize params: " + err.Error()}}
		}
		return &Response{Result: map[string]interface{}{
			"protocolVersion": version,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "wedra", "version": Version},
		}}
	case "ping":
		return &Response{Result: map[string]interface{}{}}
	case "tools/list":
		return &Response{Result: map[string]interface{}{"tools": toolDefs()}}
	case "tools/call":
		var p struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if req.Params != nil {
			_ = json.Unmarshal(req.Params, &p)
		}
		result, isErr, rpcErr := s.callTool(p.Name, p.Arguments)
		if rpcErr != nil {
			return &Response{Error: rpcErr}
		}
		return &Response{Result: map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": result}}, "isError": isErr}}
	default:
		return &Response{Error: &RPCError{Code: -32601, Message: "unknown method: " + req.Method}}
	}
}

func rpcErr(code, msg string) *RPCError {
	if strings.HasPrefix(msg, "E_") || strings.HasPrefix(code, "E_") {
		if code == "" {
			for _, known := range []string{"E_PLUGIN_OUTSIDE_ROOT", "E_FILE_REF_OUTSIDE_ROOT", "E_FILE_REF_UNCHECKED", "E_NETWORK_DENIED", "E_PLUGIN_LOAD"} {
				if strings.HasPrefix(msg, known+":") {
					code = known
					break
				}
			}
		}
		return &RPCError{Code: -32000, Message: msg, Data: map[string]string{"code": code}}
	}
	return &RPCError{Code: -32602, Message: msg}
}

func (s *Server) callTool(name string, args map[string]interface{}) (string, bool, *RPCError) {
	if args == nil {
		args = map[string]interface{}{}
	}
	switch name {
	case "list_plugins":
		return s.toolListPlugins(args)
	case "describe_plugin":
		return s.toolDescribePlugin(args)
	case "validate_pipeline":
		return s.toolValidate(args)
	case "plan_pipeline":
		return s.toolPlan(args)
	case "run_pipeline":
		return s.toolRun(args)
	case "get_run":
		return s.toolGetRun(args)
	case "cancel_run":
		return s.toolCancel(args)
	default:
		return "", false, &RPCError{Code: -32601, Message: "unknown tool: " + name}
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func permissionsJSON(m *pipeline.Manifest) map[string]interface{} {
	network := make([]map[string]interface{}, 0, len(m.Permissions.Network))
	for _, np := range m.Permissions.Network {
		network = append(network, map[string]interface{}{
			"host": np.Host, "port": np.Port, "any_host": np.AnyHost, "note": np.Note,
		})
	}
	return map[string]interface{}{
		"network": network, "filesystem": m.Permissions.Filesystem, "secrets": m.Permissions.Secrets,
	}
}

func (s *Server) toolListPlugins(args map[string]interface{}) (string, bool, *RPCError) {
	filter, _ := args["filter"].(string)
	seen := map[string]bool{}
	var list []map[string]interface{}
	for _, base := range append(append([]string{}, s.pluginsDirs...), filepath.Join(s.workDir, "plugins")) {
		for _, sub := range []string{base, filepath.Join(base, "official"), filepath.Join(base, "community")} {
			if err := s.multi.checkPluginDir(sub); err != nil {
				continue
			}
			entries, _ := os.ReadDir(sub)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(sub, e.Name())
				if err := s.multi.checkPluginDir(dir); err != nil {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
				if err != nil {
					continue
				}
				m, err := parseManifestBytes(raw)
				if err != nil {
					continue
				}
				m.Dir = dir
				if m.ID == "" {
					m.ID = e.Name()
				}
				if seen[m.ID] {
					continue
				}
				if filter != "" && !strings.Contains(m.ID, filter) && !strings.Contains(m.Description, filter) {
					continue
				}
				seen[m.ID] = true
				list = append(list, map[string]interface{}{
					"id": m.ID, "version": m.Version, "description": m.Description,
					"input": m.Input, "output": m.Output, "permissions": permissionsJSON(m),
				})
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["id"].(string) < list[j]["id"].(string) })
	return toJSON(map[string]interface{}{"plugins": list}), false, nil
}

func (s *Server) toolDescribePlugin(args map[string]interface{}) (string, bool, *RPCError) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", false, rpcErr("", "нужен id")
	}
	if err := s.checkPluginRef(id); err != nil {
		return "", false, rpcErr("E_PLUGIN_OUTSIDE_ROOT", err.Error())
	}
	m, err := s.multi.LoadManifest(id)
	if err != nil {
		return "", false, rpcErr("", err.Error())
	}
	// secrets — только имена (значения никогда не отдаём)
	return toJSON(map[string]interface{}{
		"id": m.ID, "version": m.Version, "description": m.Description, "author": m.Author,
		"runtime": m.Runtime, "input": m.Input, "output": m.Output,
		"permissions": permissionsJSON(m),
	}), false, nil
}

func (s *Server) toolValidate(args map[string]interface{}) (string, bool, *RPCError) {
	_, issues, err := s.validateArgs(args)
	if err != nil {
		if strings.HasPrefix(err.Error(), "E_PLUGIN_OUTSIDE_ROOT") {
			return "", false, rpcErr("E_PLUGIN_OUTSIDE_ROOT", err.Error())
		}
		return "", false, rpcErr("", err.Error())
	}
	errs, _ := pipeline.SplitIssues(issues)
	return toJSON(map[string]interface{}{"ok": len(errs) == 0, "issues": issues}), false, nil
}

func (s *Server) toolPlan(args map[string]interface{}) (string, bool, *RPCError) {
	pf, issues, err := s.validateArgs(args)
	if err != nil {
		if strings.HasPrefix(err.Error(), "E_PLUGIN_OUTSIDE_ROOT") {
			return "", false, rpcErr("E_PLUGIN_OUTSIDE_ROOT", err.Error())
		}
		return "", false, rpcErr("", err.Error())
	}
	plan, err := pipeline.PlanPipeline(pf, s.multi)
	if err != nil {
		return "", false, rpcErr("", err.Error())
	}
	errs, _ := pipeline.SplitIssues(issues)
	return toJSON(map[string]interface{}{
		"ok": len(errs) == 0, "issues": issues,
		"pipeline": pf.Pipeline.Name, "foreach": pf.Pipeline.Foreach,
		"dag": plan.DAG,
	}), false, nil
}

func (s *Server) toolRun(args map[string]interface{}) (string, bool, *RPCError) {
	pf, issues, err := s.validateArgs(args)
	if err != nil {
		if strings.HasPrefix(err.Error(), "E_PLUGIN_OUTSIDE_ROOT") {
			return "", false, rpcErr("E_PLUGIN_OUTSIDE_ROOT", err.Error())
		}
		return "", false, rpcErr("", err.Error())
	}
	errs, _ := pipeline.SplitIssues(issues)
	if len(errs) > 0 {
		return toJSON(map[string]interface{}{"ok": false, "issues": issues}), false, nil
	}
	waitSec := 0.0
	if w, ok := args["wait_seconds"].(float64); ok {
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 || w > maxWaitSeconds {
			return "", false, rpcErr("", "wait_seconds должен быть от 0 до 300")
		}
		waitSec = w
	}
	// secrets — только имена, значений нет; missing → понятная ошибка
	var missing []string
	for _, k := range pf.Pipeline.Secrets {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return "", false, rpcErr("secrets_missing", "secrets: нет env "+strings.Join(missing, ", ")+" (значения в YAML не живут)")
	}
	hasGate := false
	for _, st := range pf.Pipeline.Steps {
		if pipeline.IsBuiltin(st.Plugin) {
			hasGate = true
			break
		}
	}
	if hasGate && s.human == nil {
		return "", false, &RPCError{Code: -32000,
			Message: "в пайплайне есть human_gate, а канала к человеку нет (wedra mcp запущен с --no-gui): одобрить гейт некому",
			Data:    map[string]string{"code": "E_NO_HUMAN_CHANNEL"}}
	}
	runID, err := execution.NewRunID(pf.Pipeline.Name)
	if err != nil {
		return "", false, rpcErr("", "не удалось создать run_id: "+err.Error())
	}
	s.mu.Lock()
	if s.running {
		cur := s.currentRunID
		s.mu.Unlock()
		return "", false, &RPCError{Code: -32000, Message: "уже идёт ран " + cur, Data: map[string]string{"code": "E_RUN_BUSY", "run_id": cur}}
	}
	s.running = true
	s.currentRunID = runID
	s.pruneRunsLocked()
	st := &runState{id: runID, dir: filepath.Join(s.runsDir, runID), done: make(chan struct{}), status: "running"}
	s.runs[runID] = st
	s.mu.Unlock()

	go func() {
		defer close(st.done)
		defer func() {
			s.mu.Lock()
			s.running = false
			s.currentRunID = ""
			delete(s.cancels, runID)
			s.mu.Unlock()
		}()
		// Фаза 4.2: Quiet + ChannelUI (StdinUI в MCP запрещён), NoAutoApprove всегда.
		// Фаза 5: ctx для cancel_run.
		runCtx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.cancels[runID] = cancel
		s.mu.Unlock()
		defer cancel()
		ui := gate.NewChannelUI()
		// отмена закрывает гейт (иначе waiting_human висит вечно)
		go func() {
			<-runCtx.Done()
			ui.Close()
		}()
		if s.human != nil {
			// человек может отменить ран из консоли; гейт регистрируется
			// лениво — при первом gate-шаге (GateUI-фабрика ниже)
			s.human.AttachRun(runID, nil, cancel)
			defer s.human.DetachRun(runID)
		}
		gateShown := false
		opts := execution.RunOptions{
			Yes: false, Quiet: true, RunsDir: s.runsDir, RunID: runID,
			NoAutoApprove: true, MCPMode: true, Ctx: runCtx,
		}
		if s.human != nil {
			opts.GateUI = func(*pipeline.Step) gate.GateUI {
				s.human.AttachRun(runID, ui, nil)
				if !gateShown {
					gateShown = true
					s.human.GateWaiting(runID)
				}
				return ui
			}
		}
		stats, err := execution.Run(pf, s.multi, opts)
		s.mu.Lock()
		defer s.mu.Unlock()
		st.aborted = stats.Aborted
		st.okItems = stats.OK
		if err != nil {
			st.code = execution.ErrorCode(err)
			if st.code == "cancelled" {
				st.status = "cancelled"
			} else {
				st.status = "failed"
			}
			st.errMsg = err.Error()
			return
		}
		st.status = "done"
	}()

	// wait_seconds: подождать завершения/гейта
	status := "running"
	if waitSec > 0 {
		deadline := time.Now().Add(time.Duration(waitSec * float64(time.Second)))
		for time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			status = s.runStatus(runID)
			if status == "done" || status == "failed" || status == "cancelled" || status == "waiting_human" {
				break
			}
		}
	} else {
		time.Sleep(300 * time.Millisecond)
		status = s.runStatus(runID)
	}
	return toJSON(map[string]interface{}{"run_id": runID, "status": status}), false, nil
}

func (s *Server) pruneRunsLocked() {
	if len(s.runs) < maxRetainedRuns {
		return
	}
	for id, st := range s.runs {
		select {
		case <-st.done:
			delete(s.runs, id)
		default:
		}
		if len(s.runs) < maxRetainedRuns {
			return
		}
	}
}

func (s *Server) runStatus(runID string) string {
	s.mu.Lock()
	st, ok := s.runs[runID]
	s.mu.Unlock()
	if !ok {
		return "unknown"
	}
	select {
	case <-st.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return st.status
	default:
	}
	dir, err := journal.SafeRunDir(s.runsDir, runID)
	if err != nil {
		return "unknown"
	}
	rd := journal.NewReader(dir)
	events, err := rd.Events()
	if err != nil {
		return "running"
	}
	pending := false
	for _, e := range events {
		switch e["type"] {
		case "gate_wait":
			pending = true
		case "gate_decision":
			pending = false
		case "run_end":
			s.mu.Lock()
			st.status = "done"
			s.mu.Unlock()
			return "done"
		case "run_cancelled":
			s.mu.Lock()
			st.status = "cancelled"
			if msg, _ := e["error"].(string); msg != "" {
				st.errMsg = msg
			}
			s.mu.Unlock()
			return "cancelled"
		case "run_failed":
			s.mu.Lock()
			st.status = "failed"
			if msg, _ := e["error"].(string); msg != "" {
				st.errMsg = msg
			}
			s.mu.Unlock()
			return "failed"
		}
	}
	if pending {
		return "waiting_human"
	}
	return "running"
}

func truncateJSON(v interface{}, limit int) interface{} {
	b, _ := json.Marshal(v)
	if len(b) <= limit {
		return v
	}
	return common.Truncate(string(b), limit) + "(обрезано до 20KB)"
}

func (s *Server) toolGetRun(args map[string]interface{}) (string, bool, *RPCError) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return "", false, rpcErr("", "нужен run_id")
	}
	dir, err := journal.SafeRunDir(s.runsDir, runID)
	if err != nil {
		return "", false, rpcErr("", "небезопасный run_id")
	}
	s.mu.Lock()
	_, ok := s.runs[runID]
	s.mu.Unlock()
	if !ok {
		// может, ран из прошлой сессии процесса? проверяем папку
		if _, err := os.Stat(dir); err != nil {
			return "", false, rpcErr("", "ран не найден: "+runID)
		}
	}
	since := 0
	if f, ok := args["since"].(float64); ok {
		since = int(f)
	}
	rd := journal.NewReader(dir)
	events, err := rd.Events()
	if err != nil {
		return "", false, rpcErr("", err.Error())
	}
	if since < 0 {
		since = 0
	}
	if since > len(events) {
		since = len(events)
	}
	newEvents := events[since:]
	status := s.runStatus(runID)
	out := map[string]interface{}{"run_id": runID, "status": status, "total": len(events), "events": newEvents}
	s.mu.Lock()
	if rs, ok := s.runs[runID]; ok {
		select {
		case <-rs.done:
			out["stats"] = map[string]int{"ok": rs.okItems, "aborted": rs.aborted}
			if rs.errMsg != "" {
				out["error"] = rs.errMsg
				out["code"] = rs.code
			}
		default:
		}
	}
	s.mu.Unlock()
	if status == "waiting_human" {
		var lastWait map[string]interface{}
		for _, e := range events {
			if e["type"] == "gate_wait" {
				lastWait = e
			}
		}
		if lastWait != nil {
			out["pending_gate"] = map[string]interface{}{
				"step": lastWait["step"], "form": lastWait["form"], "actions": lastWait["actions"],
			}
			hint := "попросите пользователя одобрить шаг в окне wedra (у агента нет инструмента одобрения)"
			if s.human != nil {
				if u := s.human.PublicURL(runID); u != "" {
					hint += "; консоль: " + u + " (вкладка уже открыта у человека)"
				}
			}
			out["hint"] = hint
		}
	}
	if snap, err := rd.ContextSnapshot(); err == nil && snap != nil {
		out["outputs"] = truncateJSON(snap, 20*1024)
	}
	// secrets никогда не попадают: в снапшоте их нет (только input/steps)
	return toJSON(out), false, nil
}

func (s *Server) toolCancel(args map[string]interface{}) (string, bool, *RPCError) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return "", false, rpcErr("", "нужен run_id")
	}
	if _, err := journal.SafeRunDir(s.runsDir, runID); err != nil {
		return "", false, rpcErr("", "небезопасный run_id")
	}
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	_, known := s.runs[runID]
	s.mu.Unlock()
	if !known {
		return "", false, rpcErr("", "ран не найден: "+runID)
	}
	if !ok || cancel == nil {
		return "", false, &RPCError{Code: -32000, Message: "ран уже завершён", Data: map[string]string{"code": "E_RUN_DONE"}}
	}
	cancel()
	return toJSON(map[string]interface{}{"run_id": runID, "status": "cancelling"}), false, nil
}
