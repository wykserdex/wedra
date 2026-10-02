package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Контракт, который закрывает прореху в сетевой политике: sandbox исполняет
// только явный any_host. Проверяется здесь, на модели, а не только в
// платформенных бэкендах — чтобы ни один из них не смог вернуть
// «сеть есть, если что-то объявлено».
func TestNetworkRequestsAnyHost(t *testing.T) {
	cases := []struct {
		name string
		perm *[]NetworkPermission
		want bool
	}{
		{"нет объявлений", nil, false},
		{"конкретный хост", &[]NetworkPermission{{Host: "api.example.com", Port: 443}}, false},
		{"два конкретных хоста", &[]NetworkPermission{{Host: "a.example.com", Port: 443}, {Host: "b.example.com", Port: 80}}, false},
		{"явный any_host", &[]NetworkPermission{{AnyHost: true}}, true},
		{"any_host вместе с хостами", &[]NetworkPermission{{Host: "a.example.com", Port: 443}, {AnyHost: true}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{}
			if tc.perm != nil {
				m.Permissions.Network = *tc.perm
			}
			if got := NetworkRequestsAnyHost(m); got != tc.want {
				t.Errorf("NetworkRequestsAnyHost = %v, хотели %v", got, tc.want)
			}
		})
	}
}

func TestNetworkHasStructuredDeclarations(t *testing.T) {
	cases := []struct {
		name string
		perm *[]NetworkPermission
		want bool
	}{
		{"нет объявлений", nil, false},
		{"конкретный хост — неисполнимо", &[]NetworkPermission{{Host: "api.example.com", Port: 443}}, true},
		{"any_host — исполнимо", &[]NetworkPermission{{AnyHost: true}}, false},
		{"any_host перекрывает список", &[]NetworkPermission{{Host: "a.example.com", Port: 443}, {AnyHost: true}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{}
			if tc.perm != nil {
				m.Permissions.Network = *tc.perm
			}
			if got := NetworkHasStructuredDeclarations(m); got != tc.want {
				t.Errorf("NetworkHasStructuredDeclarations = %v, хотели %v", got, tc.want)
			}
		})
	}
}

// nil-манифест не должен паниковать: оба хелпера вызываются на пути сборки
// команды песочницы.
func TestNetworkHelpersNilManifest(t *testing.T) {
	if NetworkRequestsAnyHost(nil) {
		t.Error("nil-манифест не запрашивает сеть")
	}
	if NetworkHasStructuredDeclarations(nil) {
		t.Error("nil-манифест не имеет структурированных объявлений")
	}
}

// NetworkHostList по-прежнему описывает намерение автора, any_host → "*".
// Этот вывод используется в сообщениях об ошибках, поэтому он должен
// показывать именно то, что написано в манифесте.
func TestNetworkHostListKeepsDeclaredShape(t *testing.T) {
	m := &Manifest{Permissions: Permissions{Network: []NetworkPermission{
		{Host: "api.example.com", Port: 443},
		{AnyHost: true},
	}}}
	got := NetworkHostList(m)
	want := []string{"api.example.com:443", "*"}
	if len(got) != len(want) {
		t.Fatalf("NetworkHostList = %v, хотели %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NetworkHostList[%d] = %q, хотели %q", i, got[i], want[i])
		}
	}
}

// Предупреждение об исполнимости сети: автор плагина обязан узнать о ловушке
// там, где пишет манифест, а не на `pipeline run`, когда пайплайн уже собран.
// Раньше CONTRIBUTING прямо требовал объявлять `{host, port}`, а рантайм такие
// пайплайны отвергал — то есть инструкция вела в тупик.
func TestPluginManifestWarningsNetwork(t *testing.T) {
	cases := []struct {
		name    string
		network string
		want    bool
	}{
		{"сети нет — молчание", "  network: []\n", false},
		{"any_host исполнимо — молчание",
			"  network: [ { any_host: true, port: 443, note: \"target: api.example.com:443\" } ]\n", false},
		{"host:port без any_host — предупреждение",
			"  network: [ { host: \"api.example.com\", port: 443 } ]\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			manifest := "id: probe\nversion: 0.1.0\nplatform_api: \"^0.1\"\n" +
				"runtime:\n  type: python\n  entry: main.py\n" +
				"input:\n  text:\n    from: input.text\n    type: string\n" +
				"output:\n  result: { type: string }\n" +
				"permissions:\n" + tc.network + "  filesystem: none\n  secrets: []\n"
			if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			warns := PluginManifestWarnings(dir)
			if tc.want && len(warns) == 0 {
				t.Fatalf("предупреждения нет, а объявление неисполнимо:\n%s", manifest)
			}
			if !tc.want && len(warns) != 0 {
				t.Fatalf("лишнее предупреждение: %v", warns)
			}
			for _, w := range warns {
				if !strings.Contains(w, "E_NETWORK_NOT_ENFORCEABLE") {
					t.Fatalf("предупреждение не называет исход (E_NETWORK_NOT_ENFORCEABLE): %q", w)
				}
			}
		})
	}
}

// Предупреждение не должно быть ошибкой: манифест валиден, проблема в том, что
// такую сеть нельзя исполнить. Иначе автор будет править валидный файл.
func TestPluginManifestWarningsDoNotFailValidation(t *testing.T) {
	dir := t.TempDir()
	manifest := "id: probe\nversion: 0.1.0\nplatform_api: \"^0.1\"\n" +
		"runtime:\n  type: python\n  entry: main.py\n" +
		"input:\n  text:\n    from: input.text\n    type: string\n" +
		"output:\n  result: { type: string }\n" +
		"permissions:\n  network: [ { host: \"api.example.com\", port: 443 } ]\n  filesystem: none\n  secrets: []\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('{}')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if errs := ValidatePluginDir(dir); len(errs) != 0 {
		t.Fatalf("валидный манифест обязан проходить: %v", errs)
	}
	if len(PluginManifestWarnings(dir)) == 0 {
		t.Fatal("предупреждение обязано быть")
	}
}
