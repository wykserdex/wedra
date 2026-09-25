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
	"strings"
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

func TestCSRFSameSiteWithSessionCookieRejected(t *testing.T) {
	dir := t.TempDir()
	srv := NewServer(filepath.Join(dir, "plugins"), filepath.Join(dir, "pipelines"), filepath.Join(dir, "runs"))
	srv.EnableSession("session-secret")
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8765/api/run", bytes.NewReader([]byte(`{"file":"x.yaml","yes":true}`)))
	req.Header.Set("Origin", "http://127.0.0.1:8766")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("same-site session request: code=%d (want 403), body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunTraversalBlocked(t *testing.T) {
	handler, outside := secServer(t)
	root := filepath.Dir(outside)
	runs := filepath.Join(root, "runs")
	outsideRun := filepath.Join(root, "outside")
	if err := os.MkdirAll(runs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideRun, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideRun, "journal.jsonl"), []byte("{\"type\":\"run_end\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		target string
	}{
		{"detail-forward", "GET", "http://x/api/runs/..%2Foutside"},
		{"detail-backslash", "GET", `http://x/api/runs/..%5Coutside`},
		{"detail-literal-backslash", "GET", `http://x/api/runs/..\outside`},
		{"journal-forward", "GET", "http://x/api/runs/..%2Foutside/journal"},
		{"journal-backslash", "GET", `http://x/api/runs/..%5Coutside/journal`},
		{"gate-forward", "GET", "http://x/api/runs/..%2Foutside/gate"},
		{"gate-backslash", "GET", `http://x/api/runs/..%5Coutside/gate`},
		{"cancel-forward", "POST", "http://x/api/runs/..%2Foutside/cancel"},
		{"cancel-backslash", "POST", `http://x/api/runs/..%5Coutside/cancel`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != 400 {
				t.Fatalf("%s %s: code=%d body=%s", tc.method, tc.target, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "run_end") {
				t.Fatalf("traversal response leaked journal: %s", rec.Body.String())
			}
		})
	}
	validRun := filepath.Join(runs, "valid-run")
	if err := os.MkdirAll(validRun, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validRun, "journal.jsonl"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("GET", "http://x/api/runs/valid-run", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("valid run: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPipelineSymlinkBlocked(t *testing.T) {
	handler, outside := secServer(t)
	pipelines := filepath.Join(filepath.Dir(outside), "pipelines")
	link := filepath.Join(pipelines, "linked.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	req, _ := http.NewRequest("GET", "http://x/api/pipelines/linked.yaml", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("symlink pipeline: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequestBodyLimit(t *testing.T) {
	handler, _ := secServer(t)
	body := bytes.Repeat([]byte("x"), maxRequestBodySize+1)
	req, _ := http.NewRequest("PUT", "http://x/api/pipelines/large.yaml", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("oversized body: code=%d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler, _ := secServer(t)
	req, _ := http.NewRequest("GET", "http://x/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") || !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("CSP=%q", got)
	}
	for _, key := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if rec.Header().Get(key) == "" {
			t.Fatalf("missing security header %s", key)
		}
	}
}

func TestPipelinePutRejectsInvalidPolicy(t *testing.T) {
	handler, _ := secServer(t)
	body := []byte("format_version: \"0.2\"\npipeline:\n  name: bad_policy\n  gates: typo_policy\n  steps:\n    - id: review\n      plugin: core/human_gate\n")
	req, _ := http.NewRequest("PUT", "http://x/api/pipelines/policy.yaml", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("invalid policy PUT: code=%d body=%s", rec.Code, rec.Body.String())
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

	rec = post(func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "same-site")
		r.Header.Set("Origin", "http://127.0.0.1:8766")
	})
	if rec.Code != 403 {
		t.Fatalf("SFS same-site: code=%d (want 403)", rec.Code)
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
