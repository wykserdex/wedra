package api

// Тесты GET /api/plugins.
//
// Роут кормит палитру плагинов в редакторе (web/static/editor/app.js) и
// единственный вызов, которым пользуется человек. До этого у него не было НИ
// ОДНОГО теста: единственное упоминание /api/plugins в *_test.go было
// маркерной строкой в gui_embed_test.go. При этом список собирается в шести
// местах проекта, и расхождение между ними невозможно заметить без теста.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"wedra/internal/plugin"
)

// writePluginDir кладёт каталог с манифестом. Пустой plugin.yaml достаточно:
// handlePlugins проверяет только наличие файла, а полную валидацию делает
// plugin validate — здесь она лишняя и замедлила бы тест.
func writePluginDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: " + name + "\nversion: 0.1.0\nplatform_api: \"0.1\"\n" +
		"runtime:\n  type: python\n  entry: main.py\ninput: {}\noutput: {}\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func pluginsAPI(t *testing.T, pluginsDir string) []map[string]interface{} {
	t.Helper()
	// Ходим через настоящий httptest-сервер, а не через вызов хендлера
	// напрямую: так проверяется и код роута, и код ответа, как их увидит
	// браузер редактора.
	dir := t.TempDir()
	srv := NewServer(pluginsDir, filepath.Join(dir, "pipelines"), filepath.Join(dir, "runs"))
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/plugins")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/plugins = %d, ждали 200", resp.StatusCode)
	}
	var out []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("ответ не разбирается как JSON-массив: %v", err)
	}
	return out
}

func pluginIDs(list []map[string]interface{}) map[string]bool {
	ids := map[string]bool{}
	for _, p := range list {
		if id, ok := p["id"].(string); ok {
			ids[id] = true
		}
	}
	return ids
}

func TestAPIPluginsListsAllThreeRoots(t *testing.T) {
	root := t.TempDir()
	writePluginDir(t, root, "loose")
	writePluginDir(t, filepath.Join(root, "official"), "off")
	writePluginDir(t, filepath.Join(root, "community"), "com")

	got := pluginIDs(pluginsAPI(t, root))
	for _, want := range []string{"loose", "off", "com"} {
		if !got[want] {
			t.Errorf("плагин %q не попал в список: %v", want, got)
		}
	}
}

// Каталог без plugin.yaml — не плагин. Иначе в палитре появляются пустые
// серые прямоугольники, а validate потом ругается на отсутствующий манифест.
func TestAPIPluginsSkipsDirWithoutManifest(t *testing.T) {
	root := t.TempDir()
	writePluginDir(t, root, "real")
	if err := os.MkdirAll(filepath.Join(root, "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := pluginIDs(pluginsAPI(t, root))
	if got["not-a-plugin"] {
		t.Error("каталог без plugin.yaml попал в список плагинов")
	}
	if !got["real"] {
		t.Error("настоящий плагин потерялся")
	}
}

// Файл рядом с каталогами плагинов — не плагин, даже если он назван как
// плагин. Иначе одинокий plugin.yaml в корне plugins/ добавит в палитру
// запись, для которой нечего открывать.
func TestAPIPluginsSkipsFileThatLooksLikeManifest(t *testing.T) {
	root := t.TempDir()
	writePluginDir(t, root, "real")
	loose := filepath.Join(root, "stray")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loose, "plugin.yaml"),
		[]byte("id: stray\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := pluginIDs(pluginsAPI(t, root))
	if got["stray"] {
		t.Error("файл, не являющийся каталогом, попал в список плагинов")
	}
}

// Один и тот же плагин может лежать в корне и в official/. Дедуп по id
// обязателен: иначе палитра редактора покажет его дважды.
func TestAPIPluginsDedupesByID(t *testing.T) {
	root := t.TempDir()
	writePluginDir(t, root, "dupe")
	// Тот же id в official: манифест другой плагина с тем же именем.
	off := writePluginDir(t, filepath.Join(root, "official"), "dupe")
	if err := os.WriteFile(filepath.Join(off, "plugin.yaml"),
		[]byte("id: dupe\nversion: 0.2.0\nplatform_api: \"0.1\"\n"+
			"runtime:\n  type: python\n  entry: main.py\ninput: {}\noutput: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := pluginsAPI(t, root)
	count := 0
	for _, p := range list {
		if p["id"] == "dupe" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("плагин dupe встречается %d раз, ждали 1: %v", count, pluginIDs(list))
	}
}

func TestAPIPluginsEmptyRootIsNotAnError(t *testing.T) {
	got := pluginsAPI(t, t.TempDir())
	if len(got) != 0 {
		t.Errorf("пустой корень должен давать пустой список, получено %d", len(got))
	}
}

func TestAPIPluginsMissingRootIsNotFatal(t *testing.T) {
	// Отсутствующий каталог плагинов — обычное состояние (свежий checkout,
	// проект без плагинов). Роут обязан ответить 200 с пустым списком, а не
	// 500: иначе редактор не грузится вовсе.
	missing := filepath.Join(t.TempDir(), "no-such-plugins")
	got := pluginsAPI(t, missing)
	if len(got) != 0 {
		t.Errorf("несуществующий корень дал %d записей, ждали 0", len(got))
	}
}

// Каталоги агентских плагинов в списке НЕТ. Это текущее поведение, и оно
// зафиксировано здесь намеренно: AgentCanWritePlugins=false, писать
// плагины агент не может, а пока не может — незачем показывать пустую
// категорию в палитре.
//
// Если флаг включат, этот тест упадёт — и это правильный сигнал: значит
// список и документацию надо привести в соответствие.
func TestAPIPluginsOmitsAgentPluginsDir(t *testing.T) {
	root := t.TempDir()
	writePluginDir(t, root, "regular")
	writePluginDir(t, filepath.Join(root, plugin.AgentPluginDir), "mailer")

	got := pluginIDs(pluginsAPI(t, root))
	if !got["regular"] {
		t.Fatal("обычный плагин должен быть в списке")
	}
	if got["mailer"] {
		t.Errorf("плагин из %s попал в список, хотя каталог не обходится: %v",
			plugin.AgentPluginDir, got)
	}
}
