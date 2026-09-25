package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
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
	"wedra/web"
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

	// v0.9: отмена активных ранов: runID → cancel (POST /api/runs/<id>/cancel).
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc

	// v0.9: секрет сессии человека (EnableSession). Пусто — сессия не
	// требуется (тесты, встраивание). Только в памяти и в cookie.
	SessionSecret string

	summaryMu    sync.Mutex
	summaryCache map[string]summaryCacheEntry
}

type summaryCacheEntry struct {
	modTime time.Time
	size    int64
	value   map[string]interface{}
}

func NewServer(pluginsDir, pipelinesDir, runsDir string) *Server {
	eng := plugin.NewEngine()
	// v0.9: один резолв плагинов для validate/plan/list и run (раньше
	// validate жил с дефолтным "plugins", а run — с PluginsDir)
	eng.PluginsDir = pluginsDir
	return &Server{
		PluginsDir:   pluginsDir,
		PipelinesDir: pipelinesDir,
		RunsDir:      runsDir,
		Engine:       eng,
		gates:        map[string]*gate.ChannelUI{},
		cancels:      map[string]context.CancelFunc{},
		summaryCache: map[string]summaryCacheEntry{},
	}
}

// runEngine — свежий движок для рана (без кэша манифестов прошлых ранов).
func (s *Server) runEngine() *core.Engine {
	eng := core.NewEngine()
	eng.PluginsDir = s.PluginsDir
	return eng
}

func (s *Server) setCancel(id string, c context.CancelFunc) {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	s.cancels[id] = c
}

func (s *Server) clearCancel(id string) {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	delete(s.cancels, id)
}

func (s *Server) cancelFor(id string) context.CancelFunc {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	return s.cancels[id]
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

// csreHostLoopback — loopback-хост (127.0.0.1/localhost/::1) с любым портом.
func csrfHostLoopback(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i > 0 {
		h = h[:i]
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// csrfGuard — v0.28a: защита POST/PUT/DELETE от cross-site форм (<form>
// с чужого сайта запускает пайплайны с --yes):
//   - Sec-Fetch-Site: cross-site → 403 (заголовок считает браузер —
//     надёжен даже через прокси превью);
//   - Origin (если есть) → обязан совпадать с видимым host (через прокси —
//     с X-Forwarded-Host). Применяется строго к публичным биндам; на
//     loopback доверяем Sec-Fetch-Site (локальный curl без Origin — как раньше).
func (s *Server) csrfGuard(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "cross-site request запрещён (CSRF)", 403)
		return false
	}
	if !csrfHostLoopback(r.Host) {
		host := r.Host
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = strings.TrimSpace(strings.Split(fh, ",")[0])
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != host {
				http.Error(w, "Origin не совпадает (CSRF)", 403)
				return false
			}
		}
	}
	return true
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
	// static frontend — v0.7: GUI вшит в бинарник (go:embed, package web).
	// Если web/static виден из CWD (dev-режим: запуск из checkout репо) —
	// отдаём с диска (горячая правка JS без пересборки); иначе — из
	// встроенного FS: release-бинарник работает без репо на любой ОС.
	if _, err := os.Stat("web/static"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("web/static")))
	} else {
		static, err := fs.Sub(web.FS, "static")
		if err != nil {
			// недостижимо: fs.Sub по паттерну embedded-константы не падает;
			// фолбэк — честная ошибка вместо «GUI postponed»
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "GUI assets unavailable: "+err.Error(), 500)
			})
		} else {
			mux.Handle("/", http.FileServer(http.FS(static)))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if s.sessionHandshake(w, r) {
			return
		}
		if (r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE") && !s.csrfGuard(w, r) {
			return
		}
		mux.ServeHTTP(w, r)
	})
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

// networkJSON — permissions.network в нижних ключах для UI (v0.5: struct
// NetworkPermission без json-тегов, иначе Host/Port/AnyHost с капсом).
func networkJSON(list []pipeline.NetworkPermission) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(list))
	for _, np := range list {
		out = append(out, map[string]interface{}{
			"host": np.Host, "port": np.Port,
			"any_host": np.AnyHost, "note": np.Note,
		})
	}
	return out
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
	list := make([]map[string]interface{}, 0)
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
			// v0.5: редактор показывает, какие env-ключи просит плагин
			// (явные нижние ключи: у struct-полей манифеста нет json-тегов)
			"permissions": map[string]interface{}{
				"network":    networkJSON(m.Permissions.Network),
				"filesystem": m.Permissions.Filesystem,
				"secrets":    m.Permissions.Secrets,
			},
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
	// v0.28a: path traversal — Join глотает ..; без проверки читал/писал
	// любой .yaml вне PipelinesDir (GET ..%2fregistry.yaml, PUT ..%2f..%2f..)
	base := filepath.Clean(s.PipelinesDir)
	path := filepath.Join(base, name)
	if rel, err := filepath.Rel(base, path); err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.Error(w, "file: только имя из PipelinesDir (traversal запрещён)", 400)
		return
	}
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
		if !s.requireSession(w, r) {
			return
		}
		// v0.12 fix: раньше читал старый файл и писал его же обратно — молчаливая порча данных
		// теперь читаем r.Body и валидируем перед сохранением
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), 400)
			return
		}
		pf, err := pipeline.LoadPipelineFileFromBytes(data)
		if err != nil {
			http.Error(w, "invalid yaml: "+err.Error(), 400)
			return
		}
		issues := pipeline.ValidateIssues(pf, s.Engine)
		errs, _ := pipeline.SplitIssues(issues)
		if len(errs) > 0 {
			writeJSON(w, 400, map[string]interface{}{"ok": false, "issues": issues, "errors": errs})
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

func summarizeRun(dir string, events []map[string]interface{}) map[string]interface{} {
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
		case "run_resumed":
			status = "running"
		case "step_end", "step_skipped", "step_failed":
			steps++
		case "run_end":
			status = "ok"
			if ab, ok := e["aborted"].(float64); ok && ab > 0 {
				status = "aborted"
			}
		case "run_failed":
			status = "failed"
		case "run_cancelled":
			status = "cancelled"
		}
	}
	return map[string]interface{}{
		"id": filepath.Base(dir), "dir": dir, "pipeline": pipelineName,
		"status": status, "steps": steps, "events": len(events),
		"started": started, "last": last,
	}
}

func cloneSummary(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Server) cachedRunSummary(dir string) map[string]interface{} {
	path := filepath.Join(dir, "journal.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return runSummary(dir)
	}
	s.summaryMu.Lock()
	entry, ok := s.summaryCache[dir]
	s.summaryMu.Unlock()
	if ok && entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
		return cloneSummary(entry.value)
	}
	summary := runSummary(dir)
	s.summaryMu.Lock()
	if s.summaryCache == nil {
		s.summaryCache = map[string]summaryCacheEntry{}
	}
	s.summaryCache[dir] = summaryCacheEntry{modTime: info.ModTime(), size: info.Size(), value: cloneSummary(summary)}
	s.summaryMu.Unlock()
	return summary
}

func runSummary(dir string) map[string]interface{} {
	events, _ := journal.NewReader(dir).Events()
	return summarizeRun(dir, events)
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
		m := s.cachedRunSummary(filepath.Join(s.RunsDir, id))
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
	// v0.9: /api/runs/<id>/cancel — отмена активного рана (нужна сессия)
	if rest, ok := strings.CutSuffix(id, "/cancel"); ok {
		s.handleRunCancel(w, r, rest)
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
	summary := summarizeRun(dir, events)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id, "events": events, "context": snap, "artifacts": arts,
		"status": summary["status"], "pipeline": summary["pipeline"],
	})
}

func (s *Server) resolvePipelineFile(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("нужно имя или путь к pipeline")
	}
	base, err := filepath.Abs(s.PipelinesDir)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		candidates = append(candidates, name)
	} else {
		candidates = append(candidates, filepath.Join(base, name), name)
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("pipeline %q не найден внутри %s", name, s.PipelinesDir)
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
	if !s.requireSession(w, r) {
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
	path, err := s.resolvePipelineFile(req.File)
	if err != nil {
		http.Error(w, "file: "+err.Error(), 400)
		return
	}
	pf, err := pipeline.LoadPipelineFile(path)
	if err != nil {
		http.Error(w, "pipeline: "+err.Error(), 400)
		return
	}
	// v0.9: валидация и secrets — синхронно, ДО 202. Раньше 202 выдавался
	// сразу, а отказ валидации печатался только в stdout сервера: клиент
	// (агент) получал run_id несуществующего рана и не видел причину.
	eng := s.runEngine()
	issues := pipeline.ValidateIssues(pf, eng)
	errs, _ := pipeline.SplitIssues(issues)
	if len(errs) > 0 {
		writeJSON(w, 400, map[string]interface{}{"ok": false, "issues": issues})
		return
	}
	var missing []string
	for _, k := range pf.Pipeline.Secrets {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		writeJSON(w, 400, map[string]interface{}{
			"ok": false, "code": "secrets_missing", "missing": missing,
			"error": "secrets: не заданы переменные окружения: " + strings.Join(missing, ", "),
		})
		return
	}
	if !s.runMu.TryLock() {
		writeJSON(w, 409, map[string]string{"error": "уже идёт ран — дождись завершения", "code": "E_RUN_BUSY"})
		return
	}
	runID, err := execution.NewRunID(pf.Pipeline.Name)
	if err != nil {
		s.runMu.Unlock()
		writeJSON(w, 500, map[string]string{"error": "не удалось создать run_id: " + err.Error()})
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.setCancel(runID, cancel)
	s.running = true
	writeJSON(w, 202, map[string]interface{}{"status": "started", "file": req.File, "run": runID, "issues": issues})

	go func() {
		defer s.runMu.Unlock()
		defer s.clearGate(runID)
		defer s.clearCancel(runID)
		defer cancel()
		opts := core.RunOptions{Yes: req.Yes, Quiet: true, RunsDir: s.RunsDir, RunID: runID, Ctx: runCtx}
		if !req.Yes || pipelineNeedsHuman(pf) {
			opts.GateUI = func(st *pipeline.Step) gate.GateUI {
				ui := gate.NewChannelUI()
				s.setGate(runID, ui)
				// отмена рана закрывает ожидающий гейт (иначе висит вечно)
				go func() { <-runCtx.Done(); ui.Close() }()
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

// pipelineNeedsHuman — есть гейт, который --yes не одобрит (approval: human /
// gates: human_only): такому рану нужен браузерный GateUI даже при yes=true.
func pipelineNeedsHuman(pf *pipeline.PipelineFile) bool {
	for i := range pf.Pipeline.Steps {
		st := &pf.Pipeline.Steps[i]
		if pipeline.IsBuiltin(st.Plugin) && pf.Pipeline.RequiresHuman(st) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// handleRunCancel — v0.9: POST /api/runs/<id>/cancel.
func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		http.Error(w, "POST", 405)
		return
	}
	if !s.requireSession(w, r) {
		return
	}
	cancel := s.cancelFor(id)
	if cancel == nil {
		writeJSON(w, 409, map[string]string{"error": "ран не активен (завершён или не найден)", "code": "E_RUN_DONE"})
		return
	}
	cancel()
	writeJSON(w, 202, map[string]string{"status": "cancelling", "run": id})
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
		if !s.requireSession(w, r) {
			return
		}
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
		if sum := s.cachedRunSummary(dir); sum["status"] != "running" {
			st, _ := sum["status"].(string)
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]string{"error": "ран завершён (" + st + ") — решения не принимает"})
			return
		}
		// source/session проставляет сервер: клиент их не задаёт (json:"-")
		d := gate.Decision{Action: req.Action, Edits: req.Edits, Source: gate.SourceGUI, Session: s.sessionHash()}
		if !ui.SendDecision(d) {
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
	pf, err := pipeline.LoadPipelineFileFromBytes(data)
	if err != nil {
		http.Error(w, "parse: "+err.Error(), 400)
		return
	}
	issues := pipeline.ValidateIssues(pf, s.Engine)
	errs, warns := pipeline.SplitIssues(issues)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"errors": errs, "warnings": warns, "ok": len(errs) == 0, "issues": issues})
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
	issues := pipeline.ValidateIssues(pf, s.Engine)
	errs, warns := pipeline.SplitIssues(issues)
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
		"issues":   issues,
		"ok":       len(errs) == 0,
		"dag": map[string]interface{}{
			"nodes": nodes,
			"edges": edges,
		},
	})
}
