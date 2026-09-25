package cli

// F-08: direct URL install пресета.
//   - cleartext http:// запрещён (пресет = исполняемый конвейер);
//   - редирект не уводит на другую схему/host;
//   - пин #sha256=<hex> сверяется с байтами;
//   - рядом с установленным пресетом пишется .sha256 sidecar.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPresetYAML = `format_version: "0.2"
pipeline:
  name: prov
  input:
    note: hi
  steps:
    - id: review
      plugin: core/human_gate
      form:
        - { field: input.note, editable: true, type: string }
      actions: [accept, reject]
      on_reject: stop
`

// usePresetTransport — подменяет транспорт клиента загрузки на доверенный
// для httptest TLS-сервера (политика редиректов остаётся боевой).
func usePresetTransport(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := presetHTTPClient
	presetHTTPClient = func() *http.Client {
		c := newPresetClient()
		c.Transport = srv.Client().Transport
		return c
	}
	t.Cleanup(func() { presetHTTPClient = old })
}

func hexSum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// presetServer — TLS-сервер, отдающий пресет; handler задаётся тестом.
func presetServer(t *testing.T, body string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	usePresetTransport(t, srv)
	return srv
}

func TestFetchPresetRejectsCleartextAndForeignSchemes(t *testing.T) {
	for _, ref := range []string{
		"http://example.com/preset.yaml",
		"http://127.0.0.1:1/preset.yaml",
		"HTTP://example.com/preset.yaml",
		"ftp://example.com/preset.yaml",
		"file:///etc/passwd",
		"gopher://example.com/preset.yaml",
	} {
		_, _, _, err := fetchPreset(ref, "", "")
		if err == nil {
			t.Fatalf("non-https ref %q was accepted", ref)
		}
		if strings.HasPrefix(strings.ToLower(ref), "http://") && !strings.Contains(err.Error(), "cleartext") {
			t.Fatalf("cleartext %q: невнятная ошибка: %v", ref, err)
		}
	}
}

func TestFetchPresetHTTPSAllowsSameHostRedirect(t *testing.T) {
	body := validPresetYAML
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/preset.yaml":
			http.Redirect(w, r, "/moved/final.yaml", http.StatusFound)
		case "/moved/final.yaml":
			w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	raw, name, prov, err := fetchPreset(srv.URL+"/preset.yaml", "", "")
	if err != nil {
		t.Fatalf("редирект внутри host отвергнут: %v", err)
	}
	if string(raw) != body {
		t.Fatalf("content mismatch: %q", raw)
	}
	if name != "preset" {
		// имя пресета берётся из запрошенного ref (как раньше), а не из цели редиректа
		t.Fatalf("name = %q, want preset", name)
	}
	if prov.Source != srv.URL+"/moved/final.yaml" {
		t.Fatalf("provenance source = %q", prov.Source)
	}
	if prov.SourceSum != "sha256:"+hexSum([]byte(body)) {
		t.Fatalf("provenance sum = %q", prov.SourceSum)
	}
}

func TestFetchPresetHTTPSRejectsCrossHostRedirect(t *testing.T) {
	body := validPresetYAML
	evil := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer evil.Close()
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/evil.yaml", http.StatusFound)
	})
	defer srv.Close()

	_, _, _, err := fetchPreset(srv.URL+"/preset.yaml", "", "")
	if err == nil {
		t.Fatal("редирект на чужой host принят")
	}
	if !strings.Contains(err.Error(), "другой источник") {
		t.Fatalf("ожидали отказ по host, получили: %v", err)
	}
}

func TestFetchPresetHTTPSRejectsDowngradeRedirect(t *testing.T) {
	body := validPresetYAML
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer plain.Close()
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/plain.yaml", http.StatusFound)
	})
	defer srv.Close()

	_, _, _, err := fetchPreset(srv.URL+"/preset.yaml", "", "")
	if err == nil {
		t.Fatal("даунгрейд https → http принят")
	}
	if !strings.Contains(err.Error(), "редирект отклонён") || !strings.Contains(err.Error(), "cleartext") {
		t.Fatalf("ожидали отказ по схеме, получили: %v", err)
	}
}

func TestFetchPresetHTTPSRejectsRedirectChainLoop(t *testing.T) {
	body := validPresetYAML
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop.yaml", http.StatusFound)
	})
	defer srv.Close()

	_, _, _, err := fetchPreset(srv.URL+"/start.yaml", "", "")
	if err == nil {
		t.Fatal("бесконечный редирект прошёл")
	}
	if !strings.Contains(err.Error(), "редирект") {
		t.Fatalf("ожидали отказ по редиректам, получили: %v", err)
	}
}

func TestFetchPresetHTTPSVerifiesDigestPin(t *testing.T) {
	body := validPresetYAML
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pinned.yaml" {
			w.Write([]byte(body))
			return
		}
		w.Write([]byte(body + "# tampered\n"))
	})
	defer srv.Close()

	// верный пин — проходит
	ref := srv.URL + "/pinned.yaml#sha256=" + hexSum([]byte(body))
	if _, _, _, err := fetchPreset(ref, "", ""); err != nil {
		t.Fatalf("верный пин отвергнут: %v", err)
	}
	// неверный пин — отказ (файл не изменился, изменилось ожидание)
	ref = srv.URL + "/pinned.yaml#sha256=" + strings.Repeat("0", 64)
	if _, _, _, err := fetchPreset(ref, "", ""); err == nil || !strings.Contains(err.Error(), "sha256 не совпал") {
		t.Fatalf("неверный пин принят: %v", err)
	}
	// опечатка в пине — внятный отказ, а не «пин не задан»
	ref = srv.URL + "/pinned.yaml#sha256=zz"
	if _, _, _, err := fetchPreset(ref, "", ""); err == nil || !strings.Contains(err.Error(), "пин sha256") {
		t.Fatalf("битый пин принят: %v", err)
	}
	// не-pza fragment тоже fail-closed
	ref = srv.URL + "/pinned.yaml#v=2"
	if _, _, _, err := fetchPreset(ref, "", ""); err == nil {
		t.Fatal("чужой фрагмент принят молча")
	}
}

func TestFetchPresetHTTPSRedactsCredentialsInProvenance(t *testing.T) {
	body := validPresetYAML
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ref := "https://user:s3cr3t@" + u.Host + "/preset.yaml?token=s3cr3t"
	_, _, prov, err := fetchPreset(ref, "", "")
	if err != nil {
		t.Fatalf("URL с кредами отвергнут: %v", err)
	}
	if strings.Contains(prov.Source, "s3cr3t") || strings.Contains(prov.Source, "user") {
		t.Fatalf("креды попали в провенанс: %q", prov.Source)
	}
	if prov.Source != srv.URL+"/preset.yaml" {
		t.Fatalf("provenance source = %q, want %q", prov.Source, srv.URL+"/preset.yaml")
	}
}

func TestFetchPresetHTTPSKeepsRegistryAndFileFlows(t *testing.T) {
	root := t.TempDir()
	useWorkingDir(t, root)
	local := filepath.Join(root, "local.yaml")
	if err := os.WriteFile(local, []byte(validPresetYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, name, prov, err := fetchPreset("local.yaml", "", "")
	if err != nil || name != "local" {
		t.Fatalf("local file flow: %q %v", name, err)
	}
	if string(raw) != validPresetYAML {
		t.Fatal("local file content mismatch")
	}
	if prov.Source != "local.yaml" || prov.SourceSum != "sha256:"+hexSum([]byte(validPresetYAML)) {
		t.Fatalf("local provenance: %+v", prov)
	}
	// имя без источника → реестр (в cwd registry.yaml нет → дефолтный URL,
	// сюда не доходим: проверяем явный реестр)
	registryYAML := "version: \"0.1\"\npresets:\n  reg:\n    source: " + local + "\n    path: local.yaml\n"
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(registryYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, name, prov, err = fetchPreset("reg", filepath.Join(root, "registry.yaml"), "")
	if err != nil {
		t.Fatalf("registry flow: %v", err)
	}
	if name != "reg" || len(raw) == 0 {
		t.Fatalf("registry preset: %q %d bytes", name, len(raw))
	}
	if prov.Source != filepath.Clean(local) {
		t.Fatalf("registry provenance source = %q", prov.Source)
	}
}

func TestInstallPipelinePresetWritesProvenance(t *testing.T) {
	root := t.TempDir()
	useWorkingDir(t, root)
	src := filepath.Join(root, "src.yaml")
	if err := os.WriteFile(src, []byte(validPresetYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, name, prov, err := fetchPreset("src.yaml", "", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := installPipelinePreset(raw, name, "", prov)
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest == "" {
		t.Fatal("digest не записан в результат")
	}
	out := filepath.Join("examples", "prov.yaml")
	sidecar := out + presetProvenanceExt
	body, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar не записан: %v", err)
	}
	installed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := hexSum(installed) + "  prov.yaml\n"
	if !strings.HasPrefix(string(body), wantFirst) {
		t.Fatalf("sidecar:\n%s\nwant first line:\n%s", body, wantFirst)
	}
	if !strings.Contains(string(body), "# source: src.yaml") {
		t.Fatalf("в sidecar нет источника:\n%s", body)
	}
	if !strings.Contains(string(body), "# source_sha256: sha256:"+hexSum([]byte(validPresetYAML))) {
		t.Fatalf("в sidecar нет суммы источника:\n%s", body)
	}
	if err := verifyPresetProvenance(out); err != nil {
		t.Fatalf("провенанс не сходится сразу после записи: %v", err)
	}
	// повторная установка без правок — предупреждений нет
	result, err = installPipelinePreset(raw, name, "", prov)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("повторная установка дала предупреждения: %v", result.Warnings)
	}
	// правка руками → предупреждение, но установка проходит
	if err := os.WriteFile(out, []byte(validPresetYAML+"# ручная правка\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = installPipelinePreset(raw, name, "", prov)
	if err != nil {
		t.Fatalf("правка руками должна быть предупреждением, а не отказом: %v", err)
	}
	joined := strings.Join(result.Warnings, "; ")
	if !strings.Contains(joined, "провенанс") {
		t.Fatalf("нет предупреждения о провенансе: %v", result.Warnings)
	}
}

func TestRedactProvenanceSource(t *testing.T) {
	for src, want := range map[string]string{
		"https://user:s3cr3t@example.com/x.git?token=s3cr3t": "https://example.com/x.git",
		"https://example.com/x.git":                          "https://example.com/x.git",
		"ssh://git@example.com/x.git":                        "ssh://example.com/x.git",
		`C:\repos\registry`:                                  `C:\repos\registry`,
		"  /srv/registry  ":                                  "/srv/registry",
	} {
		if got := redactProvenanceSource(src); got != want {
			t.Fatalf("redactProvenanceSource(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestInstallPresetFromHTTPSWritesProvenance(t *testing.T) {
	body := validPresetYAML
	srv := presetServer(t, body, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()
	useWorkingDir(t, t.TempDir())

	raw, name, prov, err := fetchPreset(srv.URL+"/preset.yaml", "", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := installPipelinePreset(raw, name, "", prov)
	if err != nil {
		t.Fatal(err)
	}
	if result.OutFile != filepath.Join("examples", "prov.yaml") {
		t.Fatalf("out = %q", result.OutFile)
	}
	sidecar, err := os.ReadFile(result.OutFile + presetProvenanceExt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sidecar), "# source: "+srv.URL+"/preset.yaml") {
		t.Fatalf("в sidecar нет https-источника:\n%s", sidecar)
	}
	installed, err := os.ReadFile(result.OutFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sidecar), hexSum(installed)+"  prov.yaml") {
		t.Fatalf("sidecar не сходится с установленным файлом:\n%s", sidecar)
	}
	if err := verifyPresetProvenance(result.OutFile); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyPresetProvenanceDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(out, []byte(validPresetYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writePresetProvenance(out, presetProvenance{Source: "https://example.com/p.yaml"}); err != nil {
		t.Fatal(err)
	}
	if err := verifyPresetProvenance(out); err != nil {
		t.Fatalf("чистый провенанс: %v", err)
	}
	if err := os.WriteFile(out, []byte(validPresetYAML+"# injected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyPresetProvenance(out); err == nil {
		t.Fatal("изменённый пресет прошёл проверку")
	}
	// битый sidecar — тоже отказ
	if err := os.WriteFile(out+presetProvenanceExt, []byte("не сумма\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyPresetProvenance(out); err == nil {
		t.Fatal("битый sidecar прошёл проверку")
	}
	// нет sidecar — os.ErrNotExist, вызывающий решает сам
	if err := os.Remove(out + presetProvenanceExt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out + presetProvenanceExt); !os.IsNotExist(err) {
		t.Fatalf("ожидались NotExist: %v", err)
	}
}
