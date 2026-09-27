package pipeline

import "testing"

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
