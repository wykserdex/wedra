package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"wedra/internal/core"
	"wedra/internal/execution"
	"wedra/internal/gate"
	"wedra/internal/journal"
	"wedra/internal/pipeline"
)

// Version — версия сервера в initialize (cli проставляет из VERSION).
var Version = "0.9"

const defaultProtocolVersion = "2024-11-05"

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
	// v0.9: Human — канал к человеку для гейтов и отмены (встроенный GUI).
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
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *multiEngine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	if pipeline.IsBuiltin(ref) {
		if ref == "core/human_gate" {
			return &pipeline.Manifest{ID: "core/human_gate", Version: pipeline.PlatformAPI}, nil
		}
		return nil, fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	// прямой путь к директории (абсолютный, Windows тоже): грузим plugin.yaml напрямую
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") {
		// Unix-путь на Windows заведомо вне корней — песочница уже отклонила,
		// но для честности пробуем прочитать (даст понятную ошибку)
		clean := ref
		if !filepath.IsAbs(clean) {
			return nil, fmt.Errorf("плагин %q вне корней", ref)
		}
		raw, err := os.ReadFile(filepath.Join(clean, "plugin.yaml"))
		if err != nil {
			return nil, fmt.Errorf("плагин %q: %w", ref, err)
		}
		manifest, err := parseManifestBytes(raw)
		if err != nil {
			return nil, err
		}
		manifest.Dir = clean
		return manifest, nil
	}
	// абсолютные пути и .. проверяются песочницей до загрузки
	lastErr := fmt.Errorf("плагин %q не найден", ref)
	for _, dir := range m.dirs {
		eng := core.NewEngine()
		eng.PluginsDir = dir
		if manifest, err := eng.LoadManifest(ref); err == nil {
			return manifest, nil
		} else {
			lastErr = err
		}
	}
	// fallback: workDir как корень
	eng := core.NewEngine()
	eng.PluginsDir = m.workDir
	if manifest, err := eng.LoadManifest(ref); err == nil {
		return manifest, nil
	}
	return nil, lastErr
}

// checkPluginRef — песочница: абсолютные пути и .. за пределами корней запрещены.
func (s *Server) checkPluginRef(ref string) error {
	if pipeline.IsBuiltin(ref) {
		return nil
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: путь %q выходит за корни плагинов", ref)
	}
	// Unix-абсолютные (/tmp/...) — тоже абсолютные даже на Windows
	isAbs := filepath.IsAbs(ref) || strings.HasPrefix(ref, "/")
	if isAbs {
		abs := ref
		if !filepath.IsAbs(abs) {
			// Unix-путь на Windows: считаем заведомо вне корней (корни — Windows-пути)
			return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: %q вне --plugins/--workdir", ref)
		}
		abs, _ = filepath.Abs(abs)
		for _, root := range append(append([]string{}, s.pluginsDirs...), s.workDir) {
			rel, err := filepath.Rel(root, abs)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil
			}
		}
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: %q вне --plugins/--workdir", ref)
	}
	return nil
}

// checkPathInWorkdir — file_ref и path пайплайна внутри --workdir.
func (s *Server) checkPathInWorkdir(p string) error {
	if p == "" {
		return nil
	}
	if filepath.IsAbs(p) {
		rel, err := filepath.Rel(s.workDir, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: path %q вне --workdir", p)
		}
		return nil
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("E_PLUGIN_OUTSIDE_ROOT: path %q содержит ..", p)
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
	for {
		req, err := t.Read()
		if err != nil {
			return err
		}
		if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		res := s.handle(req)
		res.ID = req.ID
		if err := t.Write(res); err != nil {
			return err
		}
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
	if strings.HasPrefix(msg, "E_PLUGIN_OUTSIDE_ROOT") || strings.HasPrefix(code, "E_") {
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
			entries, _ := os.ReadDir(sub)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(sub, e.Name())
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
		// может, это прямой путь к директории
		if _, statErr := os.Stat(filepath.Join(id, "plugin.yaml")); statErr == nil {
			if err2 := s.checkPluginRef(id); err2 != nil {
				return "", false, rpcErr("E_PLUGIN_OUTSIDE_ROOT", err2.Error())
			}
			eng := core.NewEngine()
			if m2, err3 := eng.LoadManifest(id); err3 == nil {
				m = m2
			} else {
				return "", false, rpcErr("", err.Error())
			}
		} else {
			return "", false, rpcErr("", err.Error())
		}
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
	s.mu.Lock()
	if s.running {
		cur := s.currentRunID
		s.mu.Unlock()
		return "", false, &RPCError{Code: -32000, Message: "уже идёт ран " + cur, Data: map[string]string{"code": "E_RUN_BUSY", "run_id": cur}}
	}
	s.running = true
	runID := time.Now().Format("20060102-150405") + "-" + execution.Sanitize(pf.Pipeline.Name)
	s.currentRunID = runID
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
	waitSec := 0.0
	if w, ok := args["wait_seconds"].(float64); ok {
		waitSec = w
	}
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
	// живой ран: waiting_human, если gate_wait без decision
	dir := filepath.Join(s.runsDir, runID)
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
	return string(b[:limit]) + "…(обрезано до 20KB)"
}

func (s *Server) toolGetRun(args map[string]interface{}) (string, bool, *RPCError) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return "", false, rpcErr("", "нужен run_id")
	}
	s.mu.Lock()
	_, ok := s.runs[runID]
	s.mu.Unlock()
	if !ok {
		// может, ран из прошлой сессии процесса? проверяем папку
		if _, err := os.Stat(filepath.Join(s.runsDir, runID)); err != nil {
			return "", false, rpcErr("", "ран не найден: "+runID)
		}
	}
	since := 0
	if f, ok := args["since"].(float64); ok {
		since = int(f)
	}
	dir := filepath.Join(s.runsDir, runID)
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
