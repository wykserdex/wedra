package api

// v0.28a: security — path traversal в /api/pipelines/<name> (GET/PUT) и
// CSRF на POST/PUT/DELETE (cross-site формы). Аудит 2026-09: оба подтверждены
// живыми запросами (..%2fregistry.yaml читался, PUT писал в /tmp).

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// secServer — минимальный сервер в tempdir + «жертва» вне PipelinesDir.
func secServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	pipelines := filepath.Join(dir, "pipelines")
	if err := os.MkdirAll(pipelines, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(outside, []byte("# secret registry\n"), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(filepath.Join(dir, "plugins"), pipelines, filepath.Join(dir, "runs"))
	return srv.Routes(), outside
}

func TestPipelineTraversalBlocked(t *testing.T) {
	handler, _ := secServer(t)

	// GET ..%2fregistry.yaml — раньше 200 с содержимым чужого файла
	req, _ := http.NewRequest("GET", "http://x/api/pipelines/..%2fregistry.yaml", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("GET traversal: code=%d (want 400), body=%s", rec.Code, rec.Body.String())
	}

	// PUT ..%2f..%2f..%2fpwned.yaml — раньше 200 и файл появлялся
	pwned := "pwned-wedra-test.yaml"
	req, _ = http.NewRequest("PUT", "http://x/api/pipelines/..%2f..%2f..%2f..%2f..%2f"+pwned,
		bytes.NewReader([]byte("format_version: \"0.2\"\npipeline:\n  name: x\n  steps: []\n")))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("PUT traversal: code=%d (want 400), body=%s", rec.Code, rec.Body.String())
	}

	// нормальные имена работают (регрессия: 404, а не 400)
	req, _ = http.NewRequest("GET", "http://x/api/pipelines/absent.yaml", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("GET отсутствующего: code=%d (want 404)", rec.Code)
	}
}

func TestCSRFRejected(t *testing.T) {
	handler, _ := secServer(t)
	body := []byte(`{"name":"t","input":[],"steps":[],"unsupported":[]}`)
	post := func(modify func(*http.Request)) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "http://127.0.0.1:8765/api/serialize/pipeline", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		modify(req)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 1) cross-site (Sec-Fetch-Site считает браузер) — 403 даже на loopback
	rec := post(func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.Header.Set("Origin", "http://evil.example")
	})
	if rec.Code != 403 {
		t.Fatalf("SFS cross-site: code=%d (want 403)", rec.Code)
	}

	// 2) публичный хост + чужой Origin — 403
	rec = post(func(r *http.Request) {
		r.Host = "public.example"
		r.Header.Set("Origin", "http://evil.example")
	})
	if rec.Code != 403 {
		t.Fatalf("публичный хост + evil Origin: code=%d (want 403)", rec.Code)
	}

	// 3) публичный хост + свой Origin — 200 (регрессия)
	rec = post(func(r *http.Request) {
		r.Host = "public.example"
		r.Header.Set("Origin", "http://public.example")
	})
	if rec.Code != 200 {
		t.Fatalf("свой Origin: code=%d (want 200), body=%s", rec.Code, rec.Body.String())
	}

	// 4) прокси: Host loopback + X-Forwarded-Host + Origin прокси — 200
	rec = post(func(r *http.Request) {
		r.Header.Set("X-Forwarded-Host", "preview.e2b.app")
		r.Header.Set("Origin", "https://preview.e2b.app")
	})
	if rec.Code != 200 {
		t.Fatalf("XH-прокси: code=%d (want 200), body=%s", rec.Code, rec.Body.String())
	}

	// 5) curl без Origin/SFS (loopback) — 200 (CI живёт на curl)
	rec = post(func(*http.Request) {})
	if rec.Code != 200 {
		t.Fatalf("curl-стиль: code=%d (want 200), body=%s", rec.Code, rec.Body.String())
	}
}
