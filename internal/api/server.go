package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wedra/internal/core"
	"wedra/internal/execution"
	"wedra/internal/gate"
	"wedra/internal/journal"
	"wedra/internal/pipeline"
	"wedra/internal/plugin"
)

// Version — версия бинарника. var (не const): release-воркфлоу переопределяет
// через ldflags -X из тега сборки; фолбэк — текущая версия для локальных сборок.
var Version = "dev" // фолбэк без VERSION-файла (реальный — из CWD)/tag

type Server struct {
	PluginsDir   string
	PipelinesDir string
	RunsDir      string
	Engine       *plugin.Engine

	// v0.22: in-process запуск из GUI — один ран за раз
	runMu   sync.Mutex
	running bool

	// v0.24: ожидающие браузерные гейты активных ранов: runID → ChannelUI.
	// Заполняется лениво (когда ран доходит до gate-шага), чистится при выходе.
	gatesMu sync.Mutex
	gates   map[string]*gate.ChannelUI
}

func NewServer(pluginsDir, pipelinesDir, runsDir string) *Server {
	return &Server{
		PluginsDir:   pluginsDir,
		PipelinesDir: pipelinesDir,
		RunsDir:      runsDir,
		Engine:       plugin.NewEngine(),
		gates:        map[string]*gate.ChannelUI{},
	}
}

func (s *Server) setGate(id string, ui *gate.ChannelUI) {
	s.gatesMu.Lock()
	defer s.gatesMu.Unlock()
	s.gates[id] = ui
}

func (s *Server) clearGate(id string) {
	s.gatesMu.Lock()
	defer s.gatesMu.Unlock()
	if ui, ok := s.gates[id]; ok {
		ui.Close() // если ран умер с ожидающим гейтом — не висит в памяти
		delete(s.gates, id)
	}
}

func (s *Server) gateFor(id string) *gate.ChannelUI {
	s.gatesMu.Lock()
	defer s.gatesMu.Unlock()
	return s.gates[id]
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/plugins/", s.handlePluginDetail)
	mux.HandleFunc("/api/pipelines", s.handlePipelines)
	mux.HandleFunc("/api/pipelines/", s.handlePipelineDetail)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/run", s.handleRunStart)
	mux.HandleFunc("/api/validate/pipeline", s.handleValidatePipeline)
	mux.HandleFunc("/api/plan/pipeline", s.handlePlanPipeline)
	// v0.25: редактор — парсинг/сериализация через ядро (JS не держит YAML)
	mux.HandleFunc("/api/parse/pipeline", s.handleParsePipeline)
	mux.HandleFunc("/api/serialize/pipeline", s.handleSerializePipeline)
	// static frontend — если нет web/static, отдаём 404, не падаем
	if _, err := os.Stat("web/static"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("web/static")))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("GUI postponed in v0.12 — use CLI. API at /api/health"))
		})
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// v0.12 fix: читаем VERSION файл, а не хардкод 0.11
	ver := Version
	if raw, err := os.ReadFile("VERSION"); err == nil {
		ver = strings.TrimSpace(string(raw))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": ver, "protocol": "0.2"})
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	dirs := []string{}
	for _, base := range []string{s.PluginsDir, filepath.Join(s.PluginsDir, "official"), filepath.Join(s.PluginsDir, "community")} {
		if _, err := os.Stat(base); err != nil {
			continue
		}
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(base, e.Name())
			if _, err := os.Stat(filepath.Join(dir, "plugin.yaml")); err == nil {
				dirs = append(dirs, dir)
			}
		}
	}
	seen := map[string]bool{}
	var list []map[string]interface{}
	for _, d := range dirs {
		m, err := s.Engine.LoadManifest(d)
		if err != nil {
			continue
		}
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		list = append(list, map[string]interface{}{
			"id": m.ID, "version": m.Version, "description": m.Description, "author": m.Author,
			"dir": d, "runtime": m.Runtime.Type, "input": m.Input, "output": m.Output,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handlePluginDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	for _, base := range []string{s.PluginsDir, filepath.Join(s.PluginsDir, "official"), filepath.Join(s.PluginsDir, "community")} {
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			dir := filepath.Join(base, e.Name())
			m, err := s.Engine.LoadManifest(dir)
			if err != nil {
				continue
			}
			if m.ID == id || e.Name() == id {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id": m.ID, "version": m.Version, "description": m.Description, "author": m.Author,
					"dir": dir, "runtime": m.Runtime, "input": m.Input, "output": m.Output, "permissions": m.Permissions,
				})
				return
			}
		}
	}
	http.Error(w, "not found", 404)
}

func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.PipelinesDir)
	var list []map[string]interface{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(s.PipelinesDir, e.Name())
		pf, err := pipeline.LoadPipelineFile(path)
		if err != nil {
			list = append(list, map[string]interface{}{"file": e.Name(), "error": err.Error()})
			continue
		}
		list = append(list, map[string]interface{}{"file": e.Name(), "name": pf.Pipeline.Name, "steps": len(pf.Pipeline.Steps), "foreach": pf.Pipeline.Foreach})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handlePipelineDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/pipelines/")
	if name == "" {
		http.Error(w, "missing file", 400)
		return
	}
	if !strings.HasSuffix(name, ".yaml") {
		name += ".yaml"
	}
	path := filepath.Join(s.PipelinesDir, name)
	if r.Method == "GET" {
		raw, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(raw)
		return
	}
	if r.Method == "PUT" {
		// v0.12 fix: раньше читал старый файл и писал его же обратно — молчаливая порча данных
		// теперь читаем r.Body и валидируем перед сохранением
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), 400)
			return
		}
		// валидация YAML перед сохранением
		if _, err := pipeline.LoadPipelineFileFromBytes(data); err != nil {
			http.Error(w, "invalid yaml: "+err.Error(), 400)
			return
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			http.Error(w, "write: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "file": name})
		return
	}
	http.Error(w, "method not allowed", 405)
}

// runSummary — сводка рана из журнала (v0.22: статусы для GUI).
func runSummary(dir string) map[string]interface{} {
	rd := journal.NewReader(dir)
	events, _ := rd.Events()
	pipelineName, status, started, last := "", "running", "", ""
	steps := 0
	for _, e := range events {
		if ts, ok := e["ts"].(string); ok {
			if started == "" {
				started = ts
			}
			last = ts
		}
		switch e["type"] {
		case "run_start":
			pipelineName, _ = e["pipeline"].(string)
		case "step_end", "step_skipped", "step_failed":
			steps++
		case "run_end":
			status = "ok"
			// JSON-числа приходят float64
			if ab, ok := e["aborted"].(float64); ok && ab > 0 {
				status = "aborted"
			}
		case "run_failed":
			status = "failed"
		}
	}
	return map[string]interface{}{
		"id": filepath.Base(dir), "dir": dir, "pipeline": pipelineName,
		"status": status, "steps": steps, "events": len(events),
		"started": started, "last": last,
	}
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	// v0.22: filesystem — источник правды (весь вар/runs), runs.db — только
	// дополняет (artifacts; старые индексы дособираются)
	var ids []string
	seen := map[string]bool{}
	entries, _ := os.ReadDir(s.RunsDir)
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
			seen[e.Name()] = true
		}
	}
	var artsMap map[string][]string
	dbPath := filepath.Join(s.RunsDir, "runs.db")
	if _, err := os.Stat(dbPath); err == nil {
		store := journal.NewJsonStore(s.RunsDir, dbPath)
		artsMap = map[string][]string{}
		storeIds, _ := store.ListRuns()
		for _, id := range storeIds {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
			artsMap[id], _ = store.ListArtifacts(id)
		}
	}
	sortStringsDesc(ids)
	var list []map[string]interface{}
	for _, id := range ids {
		m := runSummary(filepath.Join(s.RunsDir, id))
		if artsMap != nil {
			m["artifacts"] = artsMap[id]
		}
		list = append(list, m)
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func sortStringsDesc(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] > a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	// v0.24: /api/runs/<id>/gate — статус/решение браузерного гейта
	if rest, ok := strings.CutSuffix(id, "/gate"); ok {
		s.handleRunGate(w, r, rest)
		return
	}
	// v0.22: /api/runs/<id>/journal?since=N — live-хвост для GUI
	if rest, ok := strings.CutSuffix(id, "/journal"); ok {
		dir := filepath.Join(s.RunsDir, rest)
		rd := journal.NewReader(dir)
		events, err := rd.Events()
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		since := 0
		if v := r.URL.Query().Get("since"); v != "" {
			fmt.Sscanf(v, "%d", &since)
		}
		if since < 0 || since > len(events) {
			since = len(events)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total": len(events), "events": events[since:],
		})
		return
	}
	dir := filepath.Join(s.RunsDir, id)
	rd := journal.NewReader(dir)
	events, err := rd.Events()
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	snap, _ := rd.ContextSnapshot()
	dbPath := filepath.Join(s.RunsDir, "runs.db")
	var arts []string
	if _, err := os.Stat(dbPath); err == nil {
		store := journal.NewJsonStore(s.RunsDir, dbPath)
		arts, _ = store.ListArtifacts(id)
	} else {
		store := journal.NewFilesystemStore(s.RunsDir)
		arts, _ = store.ListArtifacts(id)
	}
	summary := runSummary(dir)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id, "events": events, "context": snap, "artifacts": arts,
		"status": summary["status"], "pipeline": summary["pipeline"],
	})
}

// handleRunStart — v0.22: POST /api/run {file, yes} — in-process запуск.
// v0.24: yes=false — человеческий гейт в браузере: ран блокируется на
// gate-шаге, решение — POST /api/runs/<runID>/gate. ID рана известен заранее
// (в ответе 202) — карточка гейта не гадает имя.
func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST {file, yes}", 405)
		return
	}
	var req struct {
		File string `json:"file"`
		Yes  bool   `json:"yes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "json: "+err.Error(), 400)
		return
	}
	if req.File == "" || strings.ContainsAny(req.File, "/\\") || strings.Contains(req.File, "..") {
		http.Error(w, "file: только имя из "+s.PipelinesDir, 400)
		return
	}
	if !s.runMu.TryLock() {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "уже идёт ран — дождись завершения"})
		return
	}
	path := filepath.Join(s.PipelinesDir, req.File)
	pf, err := pipeline.LoadPipelineFile(path)
	if err != nil {
		s.runMu.Unlock()
		http.Error(w, "pipeline: "+err.Error(), 400)
		return
	}
	runID := time.Now().Format("20060102-150405") + "-" + execution.Sanitize(pf.Pipeline.Name)
	s.running = true
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "started", "file": req.File, "run": runID})

	go func() {
		defer s.runMu.Unlock()
		defer s.clearGate(runID)
		eng := core.NewEngine()
		eng.PluginsDir = s.PluginsDir
		errs, warns := core.Validate(pf, eng)
		for _, wmsg := range warns {
			fmt.Printf("[gui] warning: %s\n", wmsg)
		}
		if len(errs) > 0 {
			fmt.Printf("[gui] запуск %s отклонён валидацией: %v\n", req.File, errs)
			return
		}
		opts := core.RunOptions{Yes: req.Yes, Quiet: true, RunsDir: s.RunsDir, RunID: runID}
		if !req.Yes {
			opts.GateUI = func(st *pipeline.Step) gate.GateUI {
				ui := gate.NewChannelUI()
				s.setGate(runID, ui)
				return ui
			}
		}
		stats, err := core.Run(pf, eng, opts)
		if err != nil {
			fmt.Printf("[gui] %s: %v\n", req.File, err)
		} else {
			fmt.Printf("[gui] %s: ok=%d aborted=%d\n", req.File, stats.OK, stats.Aborted)
		}
	}()
}

// handleRunGate — v0.24: GET/POST /api/runs/<id>/gate.
// GET — ожидающий ли гейт (из журнала: gate_wait без gate_decision после).
// POST {action, edits} — решение: POST в ChannelUI активного рана (409, если
// гейта нет, уже решён или ран завершён).
func (s *Server) handleRunGate(w http.ResponseWriter, r *http.Request, id string) {
	dir := filepath.Join(s.RunsDir, id)
	switch r.Method {
	case "GET":
		rd := journal.NewReader(dir)
		events, err := rd.Events()
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		pending := false
		var lastWait map[string]interface{}
		for _, e := range events {
			switch e["type"] {
			case "gate_wait":
				pending, lastWait = true, e
			case "gate_decision":
				pending, lastWait = false, nil
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if pending {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"pending": true, "step": lastWait["step"],
				"form": lastWait["form"], "actions": lastWait["actions"],
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"pending": false})
		}
	case "POST":
		var req struct {
			Action string                 `json:"action"`
			Edits  map[string]interface{} `json:"edits"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "json: "+err.Error(), 400)
			return
		}
		ui := s.gateFor(id)
		if ui == nil {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"error": "нет ожидающего гейта (ран не на гейте или завершён)"})
			return
		}
		// v0.27: ран завершён (run_end в журнале) — решения не принимает, даже
		// если clearGate ещё не успел сработать (окно между записью run_end и
		// dereg под нагрузкой давало 202 на мёртвый ран)
		if sum := runSummary(dir); sum["status"] != "running" {
			st, _ := sum["status"].(string)
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"error": "ран завершён (" + st + ") — решения не принимает"})
			return
		}
		if !ui.SendDecision(gate.Decision{Action: req.Action, Edits: req.Edits}) {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"error": "гейт уже решён"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]string{"status": "queued"})
	default:
		http.Error(w, "GET or POST", 405)
	}
}

func (s *Server) handleValidatePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST yaml", 405)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var pf pipeline.PipelineFile
	if err := json.Unmarshal(data, &pf); err != nil {
		pfPtr, err2 := pipeline.LoadPipelineFileFromBytes(data)
		if err2 != nil {
			http.Error(w, fmt.Sprintf("parse error: %v / %v", err, err2), 400)
			return
		}
		pf = *pfPtr
	}
	errs, warns := pipeline.Validate(&pf, s.Engine)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"errors": errs, "warnings": warns, "ok": len(errs) == 0})
}

func (s *Server) handlePlanPipeline(w http.ResponseWriter, r *http.Request) {
	// v0.12 fix: раньше был алиас на validate, никакого DAG
	// теперь строит DAG: узлы + рёбра по bind/form зависимостям
	if r.Method != "POST" {
		http.Error(w, "POST yaml", 405)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	pf, err := pipeline.LoadPipelineFileFromBytes(data)
	if err != nil {
		http.Error(w, "parse: "+err.Error(), 400)
		return
	}
	errs, warns := pipeline.Validate(pf, s.Engine)
	// строим DAG
	nodes := []map[string]interface{}{}
	edges := []map[string]string{}
	// pre-phase для steps.* foreach
	preSteps := map[string]bool{}
	if pf.Pipeline.Foreach != "" && strings.HasPrefix(pf.Pipeline.Foreach, "steps.") {
		parts := strings.Split(pf.Pipeline.Foreach, ".")
		if len(parts) >= 2 {
			srcID := parts[1]
			for _, st := range pf.Pipeline.Steps {
				preSteps[st.ID] = true
				if st.ID == srcID {
					break
				}
			}
		}
	}
	for _, st := range pf.Pipeline.Steps {
		phase := "foreach"
		if preSteps[st.ID] {
			phase = "pre"
		}
		if st.AfterForeach {
			phase = "post"
		}
		whenLabel := ""
		if st.When.IsSet() {
			whenLabel = st.When.Path
			if st.When.Op != "" && st.When.Op != "truthy" {
				whenLabel += " " + st.When.Op
				if st.When.Value != nil {
					whenLabel += " " + fmt.Sprintf("%v", st.When.Value)
				}
			}
		}
		nodes = append(nodes, map[string]interface{}{
			"id": st.ID, "plugin": st.Plugin, "phase": phase, "on_error": st.OnError,
			"bind": st.Bind, "after_foreach": st.AfterForeach,
			"when": whenLabel, "foreach": st.Foreach, "foreach_item": st.ForeachItem,
			"parallel_group": st.ParallelGroup,
		})
		// v0.22: рёбра от путей when/foreach (зависимость на данные)
		for _, p := range []string{st.Foreach, st.When.Path} {
			if strings.HasPrefix(p, "steps.") {
				parts := strings.Split(p, ".")
				if len(parts) >= 2 {
					edges = append(edges, map[string]string{"from": parts[1], "to": st.ID, "via": p})
				}
			}
		}
		// рёбра: из bind и form
		for _, from := range st.Bind {
			if strings.HasPrefix(from, "steps.") {
				parts := strings.Split(from, ".")
				if len(parts) >= 2 {
					edges = append(edges, map[string]string{"from": parts[1], "to": st.ID, "via": from})
				}
			}
		}
		for _, f := range st.Form {
			if strings.HasPrefix(f.Field, "steps.") {
				parts := strings.Split(f.Field, ".")
				if len(parts) >= 2 {
					edges = append(edges, map[string]string{"from": parts[1], "to": st.ID, "via": f.Field})
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pipeline": pf.Pipeline.Name,
		"foreach":  pf.Pipeline.Foreach,
		"errors":   errs,
		"warnings": warns,
		"ok":       len(errs) == 0,
		"dag": map[string]interface{}{
			"nodes": nodes,
			"edges": edges,
		},
	})
}
