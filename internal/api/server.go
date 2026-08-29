package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/internal/journal"
	"orchestrator/internal/pipeline"
	"orchestrator/internal/plugin"
)

const Version = "0.14.1"

type Server struct {
	PluginsDir   string
	PipelinesDir string
	RunsDir      string
	Engine       *plugin.Engine
}

func NewServer(pluginsDir, pipelinesDir, runsDir string) *Server {
	return &Server{
		PluginsDir:   pluginsDir,
		PipelinesDir: pipelinesDir,
		RunsDir:      runsDir,
		Engine:       plugin.NewEngine(),
	}
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
	mux.HandleFunc("/api/validate/pipeline", s.handleValidatePipeline)
	mux.HandleFunc("/api/plan/pipeline", s.handlePlanPipeline)
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

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	var list []map[string]interface{}
	dbPath := filepath.Join(s.RunsDir, "runs.db")
	if _, err := os.Stat(dbPath); err == nil {
		store := journal.NewJsonStore(s.RunsDir, dbPath)
		ids, _ := store.ListRuns()
		for _, id := range ids {
			dir := filepath.Join(s.RunsDir, id)
			rd := journal.NewReader(dir)
			events, _ := rd.Events()
			pipelineName := ""
			if len(events) > 0 {
				if pn, ok := events[0]["pipeline"].(string); ok {
					pipelineName = pn
				}
			}
			arts, _ := store.ListArtifacts(id)
			list = append(list, map[string]interface{}{"id": id, "dir": dir, "pipeline": pipelineName, "events": len(events), "artifacts": arts, "store": "json"})
		}
	} else {
		entries, _ := os.ReadDir(s.RunsDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(s.RunsDir, e.Name())
			rd := journal.NewReader(dir)
			events, _ := rd.Events()
			pipelineName := ""
			if len(events) > 0 {
				if pn, ok := events[0]["pipeline"].(string); ok {
					pipelineName = pn
				}
			}
			list = append(list, map[string]interface{}{"id": e.Name(), "dir": dir, "pipeline": pipelineName, "events": len(events), "store": "fs"})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if id == "" {
		http.Error(w, "missing id", 400)
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "events": events, "context": snap, "artifacts": arts})
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
		nodes = append(nodes, map[string]interface{}{
			"id": st.ID, "plugin": st.Plugin, "phase": phase, "on_error": st.OnError,
			"bind": st.Bind, "after_foreach": st.AfterForeach,
		})
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
