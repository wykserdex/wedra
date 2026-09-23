package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sessionTestServer — сервер с включённой сессией.
func sessionTestServer(t *testing.T) (string, *Server) {
	t.Helper()
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	pipelines := filepath.Join(dir, "pipelines")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{plugins, pipelines, runs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pipelines, "gate_demo.yaml"), []byte(gatePipeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(plugins, pipelines, runs)
	srv.EnableSession("test-secret-1234567890abcdef")
	ts := newTestServer(t, srv)
	return ts.URL, srv
}

func doPost(t *testing.T, url string, body interface{}, cookie string) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "wedra_session", Value: cookie})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func doGet(t *testing.T, url string, cookie string) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "wedra_session", Value: cookie})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

const testCookie = "test-secret-1234567890abcdef"

func startRunWithCookie(t *testing.T, base string) string {
	t.Helper()
	code, body := doPost(t, base+"/api/run", map[string]interface{}{"file": "gate_demo.yaml", "yes": false}, testCookie)
	if code != 202 {
		t.Fatalf("POST /api/run с cookie: code=%d body=%v (want 202)", code, body)
	}
	runID, _ := body["run"].(string)
	if runID == "" {
		t.Fatalf("нет run: %v", body)
	}
	return runID
}

func waitPendingGateDirect(t *testing.T, base, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		code, d := doGet(t, base+"/api/runs/"+id+"/gate", testCookie)
		if code == 200 && d["pending"] == true {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("гейт не в pending: code=%d body=%v", code, d)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func decideGateWithCookie(t *testing.T, base, id, action string) {
	t.Helper()
	code, _ := doPost(t, base+"/api/runs/"+id+"/gate", map[string]interface{}{"action": action}, testCookie)
	if code != 202 {
		t.Fatalf("POST gate с cookie: code=%d (want 202)", code)
	}
}

func waitRunStatusDirect(t *testing.T, base, id, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, d := doGet(t, base+"/api/runs/"+id, "")
		if d["status"] == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("статус=%v want %q", d["status"], want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func grepDir(t *testing.T, dir, secret string) []string {
	t.Helper()
	var hits []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			hits = append(hits, grepDir(t, filepath.Join(dir, e.Name()), secret)...)
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), secret) {
			hits = append(hits, filepath.Join(dir, e.Name()))
		}
	}
	return hits
}

func TestSessionGateRequiresCookie(t *testing.T) {
	base, _ := sessionTestServer(t)

	// без cookie — 401
	code, _ := doPost(t, base+"/api/run", map[string]interface{}{"file": "gate_demo.yaml", "yes": false}, "")
	if code != 401 {
		t.Fatalf("POST /api/run без cookie: code=%d (want 401)", code)
	}

	// с cookie — 202
	runID := startRunWithCookie(t, base)
	waitPendingGateDirect(t, base, runID)
	// gate без cookie — 401
	code, _ = doPost(t, base+"/api/runs/"+runID+"/gate", map[string]interface{}{"action": "accept"}, "")
	if code != 401 {
		t.Fatalf("POST gate без cookie: code=%d (want 401)", code)
	}
	// с cookie — 202 и ран завершается
	decideGateWithCookie(t, base, runID, "accept")
	waitRunStatusDirect(t, base, runID, "ok")
}

func TestSessionSecretNotInJournal(t *testing.T) {
	base, srv := sessionTestServer(t)
	runID := startRunWithCookie(t, base)
	waitPendingGateDirect(t, base, runID)
	decideGateWithCookie(t, base, runID, "accept")
	waitRunStatusDirect(t, base, runID, "ok")

	secret := srv.SessionSecret
	hits := grepDir(t, srv.RunsDir, secret)
	if len(hits) > 0 {
		t.Fatalf("секрет утёк в журнал: %v", hits)
	}
	found := false
	for _, ev := range runJournalEvents(t, srv.RunsDir, runID) {
		if ev["type"] == "gate_decision" {
			src, _ := ev["source"].(string)
			if src == "" {
				t.Fatalf("gate_decision без source: %v", ev)
			}
			if src != "gui" {
				t.Fatalf("source=%q (want gui)", src)
			}
			if _, hasSession := ev["session"]; !hasSession {
				t.Fatalf("gate_decision без session: %v", ev)
			}
			if sess, _ := ev["session"].(string); sess == secret {
				t.Fatalf("session == секрет (должен быть хэш)")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("нет gate_decision")
	}
}
