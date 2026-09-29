package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sessionCode — одноразовый код обмена в тестах (не секрет сессии: cookie
// выдаётся другой, с TTL, и им нельзя войти повторно).
const sessionCode = "TEST-TEST-TEST"

// sessionTestServer — сервер с включённой сессией и HTTP-клиентом с cookie jar:
// дальше тесты ходят как браузер (cookie сама прикладывается, имя — с портом).
func sessionTestServer(t *testing.T) (string, *http.Client, *Server) {
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
	ts := newTestServer(t, srv)
	// порт httptest неизвестен заранее — объявляем его (имя cookie с портом)
	host, port, err := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.ConfigureListen(ListenOptions{Addr: net.JoinHostPort(host, port), ResolvedPort: port}); err != nil {
		t.Fatal(err)
	}
	srv.EnableSession(sessionCode)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	// вход: одноразовый код → cookie
	resp, err := client.Post(ts.URL+"/api/session", "application/json", bytes.NewReader([]byte(`{"code":"`+sessionCode+`"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("вход по коду: code=%d body=%s", resp.StatusCode, body)
	}
	return ts.URL, client, srv
}

func doPost(t *testing.T, client *http.Client, url string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func doGet(t *testing.T, client *http.Client, url string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func startRunWithCookie(t *testing.T, base string, client *http.Client) string {
	t.Helper()
	code, body := doPost(t, client, base+"/api/run", map[string]interface{}{"file": "gate_demo.yaml", "yes": false})
	if code != 202 {
		t.Fatalf("POST /api/run с cookie: code=%d body=%v (want 202)", code, body)
	}
	runID, _ := body["run"].(string)
	if runID == "" {
		t.Fatalf("нет run: %v", body)
	}
	return runID
}

func waitPendingGateDirect(t *testing.T, base, id string, client *http.Client) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		code, d := doGet(t, client, base+"/api/runs/"+id+"/gate")
		if code == 200 && d["pending"] == true {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("гейт не в pending: code=%d body=%v", code, d)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func decideGateWithCookie(t *testing.T, base, id, action string, client *http.Client) {
	t.Helper()
	code, _ := doPost(t, client, base+"/api/runs/"+id+"/gate", map[string]interface{}{"action": action})
	if code != 202 {
		t.Fatalf("POST gate с cookie: code=%d (want 202)", code)
	}
}

func waitRunStatusDirect(t *testing.T, base, id, want string, client *http.Client) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, d := doGet(t, client, base+"/api/runs/"+id)
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
	base, client, srv := sessionTestServer(t)

	// без cookie — 401 на мутации...
	code, _ := doPost(t, anonClient(t), base+"/api/run", map[string]interface{}{"file": "gate_demo.yaml", "yes": false})
	if code != 401 {
		t.Fatalf("POST /api/run без cookie: code=%d (want 401)", code)
	}
	// ...и на чтении тоже: журналы и входы шагов больше не открыты
	code, _ = doGet(t, anonClient(t), base+"/api/runs")
	if code != 401 {
		t.Fatalf("GET /api/runs без cookie: code=%d (want 401)", code)
	}

	// с cookie — 202
	runID := startRunWithCookie(t, base, client)
	waitPendingGateDirect(t, base, runID, client)
	// решение гейта без cookie — 401
	code, _ = doPost(t, anonClient(t), base+"/api/runs/"+runID+"/gate", map[string]interface{}{"action": "accept"})
	if code != 401 {
		t.Fatalf("POST gate без cookie: code=%d (want 401)", code)
	}
	// с cookie — 202 и ран завершается
	decideGateWithCookie(t, base, runID, "accept", client)
	waitRunStatusDirect(t, base, runID, "ok", client)
	_ = srv
}

// anonClient — клиент без cookie jar (чужой локальный процесс).
func anonClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{}
}

func TestSessionSecretNotInJournal(t *testing.T) {
	base, client, srv := sessionTestServer(t)
	runID := startRunWithCookie(t, base, client)
	waitPendingGateDirect(t, base, runID, client)
	decideGateWithCookie(t, base, runID, "accept", client)
	waitRunStatusDirect(t, base, runID, "ok", client)

	// ни код, ни токен сессии в журнал не попадают
	for _, secret := range []string{sessionCode, srv.PairingCode} {
		if secret == "" {
			continue
		}
		if hits := grepDir(t, srv.RunsDir, secret); len(hits) > 0 {
			t.Fatalf("секрет утёк в журнал: %v", hits)
		}
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
			if sess, _ := ev["session"].(string); sess == srv.PairingCode {
				t.Fatalf("session == код/секрет (должен быть хэш)")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("нет gate_decision")
	}
}

// TestSessionCookieIsNotTheSecret — cookie не равна ни коду, ни секрету:
// токен с TTL в памяти сервера. Раньше cookie была самим секретом, поэтому
// любая утечка cookie была навсегда.
func TestSessionCookieIsNotTheSecret(t *testing.T) {
	base, client, srv := sessionTestServer(t)
	var token string
	for _, ck := range client.Jar.Cookies(mustURL(t, base)) {
		token = ck.Value
	}
	if token == "" {
		t.Fatal("cookie сессии не найдена")
	}
	if token == srv.PairingCode {
		t.Fatal("cookie равна коду/секрету")
	}
	if len(token) < 32 {
		t.Fatalf("токен сессии подозрительно короткий: %q", token)
	}
	// токен не в памяти как «секрет» и не переживает перезапуск: сервер без
	// состояния сессию не признаёт
	srv.EnableSession("NNNN-NNNN-NNNN")
	code, _ := doGet(t, client, base+"/api/runs")
	if code != 401 {
		t.Fatalf("после смены кода старый токен: code=%d (want 401)", code)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
