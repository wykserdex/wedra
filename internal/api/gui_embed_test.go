package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestGUIEmbeddedStatic — v0.7: GUI вшит в бинарник (go:embed).
// Тесты гонятся из internal/api — там нет web/static (он в корне репо),
// то есть этот тест проверяет именно release-путь: frontend из embedded FS,
// без диска рядом.
func TestGUIEmbeddedStatic(t *testing.T) {
	ts, _ := gateTestServer(t)

	cases := []struct {
		path string
		want string // подстрока, обязана быть в теле
	}{
		{"/", "WEDRA"},
		{"/index.html", "WEDRA"},
		{"/editor/", "редактор"},
		{"/editor/app.js", "pluginNetworkHint"}, // маркер v0.6 UI
		{"/app.js", "api/health"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s: status=%d (ждём 200 из embedded FS)", c.path, resp.StatusCode)
		}
		if !strings.Contains(string(body), c.want) {
			t.Fatalf("GET %s: нет %q в теле (embedded FS битый?)", c.path, c.want)
		}
	}

	// честный 404 на несуществующее
	resp, err := http.Get(ts.URL + "/definitely_not_there.html")
	if err != nil {
		t.Fatalf("GET 404: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("GET /definitely_not_there.html: status=%d (ждём 404)", resp.StatusCode)
	}
}
