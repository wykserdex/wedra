package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newAssetsServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	srv := NewServer(filepath.Join(root, "plugins"), filepath.Join(root, "pipes"), filepath.Join(root, "runs"))
	srv.AssetsDir = filepath.Join(root, "assets")
	ts := newTestServer(t, srv)
	return ts, srv.AssetsDir
}

// postAsset — multipart-загрузка с произвольным именем файла.
func postAsset(t *testing.T, ts *httptest.Server, field, filename string, body []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if filename == "" {
		// без файла: только поле с другим именем
		if field != "" {
			_ = mw.WriteField(field, "x")
		}
	} else {
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(body); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/assets", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("POST /api/assets: %v", err)
	}
	return resp
}

func decodeAsset(t *testing.T, resp *http.Response) assetInfo {
	t.Helper()
	defer resp.Body.Close()
	var a assetInfo
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return a
}

func TestAssetsUploadAndList(t *testing.T) {
	ts, dir := newAssetsServer(t)

	resp := postAsset(t, ts, "file", "photo.jpg", []byte("JPEGDATA"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус %d, ждём 200", resp.StatusCode)
	}
	a := decodeAsset(t, resp)
	if a.Name != "photo.jpg" {
		t.Errorf("имя = %q, ждём photo.jpg", a.Name)
	}
	// Путь обязан быть абсолютным: относительный плагин разрешит от своего
	// рабочего каталога и файла не найдёт (PROTOCOL §1).
	if !filepath.IsAbs(a.Path) {
		t.Errorf("путь %q не абсолютный — плагин его не найдёт", a.Path)
	}
	if got, _ := os.ReadFile(a.Path); string(got) != "JPEGDATA" {
		t.Errorf("содержимое = %q, ждём JPEGDATA", got)
	}
	// Файл обязан лежать внутри каталога артефактов.
	if rel, err := filepath.Rel(dir, a.Path); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("файл %q вне %q (rel=%q, err=%v)", a.Path, dir, rel, err)
	}
	// .part не должно остаться: мусор в каталоге артефактов сбивает список.
	if _, err := os.Stat(filepath.Join(dir, "photo.jpg.part")); !os.IsNotExist(err) {
		t.Errorf(".part остался в каталоге артефактов")
	}

	listResp, err := http.Get(ts.URL + "/api/assets")
	if err != nil {
		t.Fatalf("GET /api/assets: %v", err)
	}
	defer listResp.Body.Close()
	var list []assetInfo
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "photo.jpg" {
		t.Fatalf("список = %+v, ждём один photo.jpg", list)
	}
}

// Коллизия имён: второй файл с тем же именем не должен перезаписать первый.
func TestAssetsNameCollisionKeepsBoth(t *testing.T) {
	ts, _ := newAssetsServer(t)

	first := decodeAsset(t, postAsset(t, ts, "file", "photo.jpg", []byte("ONE")))
	second := decodeAsset(t, postAsset(t, ts, "file", "photo.jpg", []byte("TWO")))

	if second.Name == first.Name {
		t.Fatalf("вторый файл получил то же имя %q — первый перезаписан", second.Name)
	}
	if second.Path == first.Path {
		t.Fatalf("вторый файл лёг в тот же путь %q", second.Path)
	}
	if got, _ := os.ReadFile(first.Path); string(got) != "ONE" {
		t.Errorf("первый файл = %q, ждём ONE — его перезаписали", got)
	}
	if got, _ := os.ReadFile(second.Path); string(got) != "TWO" {
		t.Errorf("второй файл = %q, ждём TWO", got)
	}
}

// sanitizeAssetName — чистая функция, поэтому проверяем её напрямую: через
// HTTP каждый случай стоил бы отдельной загрузки, а проверять тут нечего.
func TestSanitizeAssetName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"photo.jpg", "photo.jpg"},
		{"../../etc/passwd", "passwd"},
		{"..\\..\\windows\\system32\\evil.dll", "evil.dll"},
		{"/absolute/path/photo.png", "photo.png"},
		{"C:\\Users\\me\\photo.png", "photo.png"},
		{"  spaces  .jpg", "spaces.jpg"},
		{"снег  и лёд.JPG", "sneg_i_led.jpg"}, // ё→e по таблице translit // кириллица → латиница, серия пробелов → один "_"
		// Основа съедена очисткой, но имя есть — нейтральная основа, а
		// «.jpg» вместо «jpg» сломало бы определение mime по расширению.
		{"моё фото.jpg", "moe_foto.jpg"},
		{"photo.", "photo"},
		{"a.tar.gz", "a.tar.gz"},
	}
	for _, c := range cases {
		got, err := sanitizeAssetName(c.in)
		if err != nil {
			t.Errorf("вход %q → ошибка %v, ждём %q", c.in, err, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("вход %q → %q, ждём %q", c.in, got, c.want)
		}
	}
}

// Имя без содержимого — мусор клиента, а не файл.
func TestSanitizeAssetNameRejectsNameless(t *testing.T) {
	for _, in := range []string{"", "..", ".", "./.", "   "} {
		if got, err := sanitizeAssetName(in); err == nil {
			t.Errorf("вход %q принят как %q, ждём ошибку", in, got)
		}
	}
}

// Зарезервированные имена устройств Windows: такой файл не создаётся.
func TestSanitizeAssetNameRejectsWindowsDevices(t *testing.T) {
	for _, in := range []string{"CON", "con", "PRN", "NUL", "COM1", "lpt9", "AUX"} {
		if got, err := sanitizeAssetName(in); err == nil {
			t.Errorf("вход %q принят как %q, ждём ошибку", in, got)
		}
	}
	// А вот эти — обычные имена, устройствами не являются.
	for _, in := range []string{"console.log", "com10.txt", "nullable", "auxiliary"} {
		if _, err := sanitizeAssetName(in); err != nil {
			t.Errorf("вход %q отклонён как устройство: %v", in, err)
		}
	}
}

// Проверка containment на уровне HTTP: traversal в имени не должен увести
// файл за пределы каталога артефактов.
func TestAssetsUploadContained(t *testing.T) {
	ts, dir := newAssetsServer(t)

	for _, name := range []string{"../../evil.txt", "..\\..\\evil2.txt", "/tmp/evil3.txt"} {
		a := decodeAsset(t, postAsset(t, ts, "file", name, []byte("X")))
		rel, err := filepath.Rel(dir, a.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Errorf("вход %q → %q (rel=%q, err=%v) вне %q", name, a.Path, rel, err, dir)
		}
	}
	// За пределами каталога ничего не появилось.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("файл создан на уровень выше каталога артефактов")
	}
}

func TestAssetsRejectsBadInput(t *testing.T) {
	ts, _ := newAssetsServer(t)

	// Имя, из которого после очистки ничего не остаётся.
	if resp := postAsset(t, ts, "file", "..", []byte("X")); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Errorf("имя \"..\" → статус %d, ждём 400", resp.StatusCode)
	}
	// Зарезервированное имя устройства Windows.
	if resp := postAsset(t, ts, "file", "CON", []byte("X")); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Errorf("CON → статус %d, ждём 400", resp.StatusCode)
	}
	// Не то поле формы.
	if resp := postAsset(t, ts, "attachment", "photo.jpg", []byte("X")); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Errorf("поле attachment → статус %d, ждём 400", resp.StatusCode)
	}
	// Пустой файл: в ноль байт. 400, а не 500 — это ошибка человека, а не сервера.
	if resp := postAsset(t, ts, "file", "empty.txt", nil); resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Errorf("пустой файл → статус %d, ждём 400 (тело: %s)", resp.StatusCode, body)
	}
	// Метод не тот.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/assets", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE → статус %d, ждём 405", resp.StatusCode)
	}
}

// Артефакты — содержимое входа шага, поэтому защита та же, что у пайплайнов:
// без сессии человека /api/assets быть не должно.
func TestAssetsRequiresSession(t *testing.T) {
	root := t.TempDir()
	srv := NewServer(filepath.Join(root, "plugins"), filepath.Join(root, "pipes"), filepath.Join(root, "runs"))
	srv.AssetsDir = filepath.Join(root, "assets")
	srv.EnableSession("TEST-CODE-1234")
	ts := newTestServer(t, srv)

	resp, err := http.Get(ts.URL + "/api/assets")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("GET /api/assets без сессии вернул 200 — содержимое входов утекло")
	}
}
