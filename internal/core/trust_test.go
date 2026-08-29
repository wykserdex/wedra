package core

// v0.17: trust — declare-now (network: deny + WEDRA_NETWORK) и кросс-проверка secrets.

import (
	"strings"
	"testing"
)

// ── network: deny + плагин заявил сеть → ошибка в валидаторе и раннере ──

func TestValidateNetworkDeny(t *testing.T) {
	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:    "t_net_deny",
			Input:   map[string]interface{}{"value": "x"},
			Network: "deny",
			Steps: []Step{
				{ID: "net", Plugin: fxPlugins + "net_demo", OnError: "stop"},
			},
		},
	}
	errs, _ := Validate(pf, NewEngine())
	joined := strings.Join(errs, "; ")
	if !strings.Contains(joined, "network: deny") {
		t.Fatalf("ожидалась ошибка network: deny, получили: %v", errs)
	}
}

func TestRunNetworkDenyDeclared(t *testing.T) {
	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:    "t_net_deny",
			Input:   map[string]interface{}{"value": "x"},
			Network: "deny",
			Steps: []Step{
				{ID: "net", Plugin: fxPlugins + "net_demo", OnError: "stop"},
			},
		},
	}
	_, err := Run(pf, NewEngine(), quietOpts(t))
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("ожидается network-ошибка до любого эффекта: %v", err)
	}
}

// ── WEDRA_NETWORK=deny доходит до subprocess (контракт для честных плагинов) ──

func TestRunNetworkDenyEnv(t *testing.T) {
	requirePython(t)
	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:    "t_net_env",
			Input:   map[string]interface{}{"value": "x"},
			Network: "deny",
			Steps: []Step{
				{ID: "probe", Plugin: fxPlugins + "net_probe", OnError: "stop"},
			},
		},
	}
	stats, err := Run(pf, NewEngine(), quietOpts(t))
	if err != nil {
		t.Fatalf("ран упал: %v", err)
	}
	snap := readContextSnapshot(t, stats.RunDir)
	steps := snap["steps"].(map[string]interface{})
	if got := steps["probe"].(map[string]interface{})["value"]; got != "deny" {
		t.Fatalf("WEDRA_NETWORK=deny не дошёл до subprocess: %v", got)
	}
}

// ── secrets: кросс-проверка pipeline.secrets ↔ permissions.secrets ──

func TestValidateSecretsCrossCheck(t *testing.T) {
	// плагин net_demo заявляет NET_DEMO_KEY; пайплайн не объявляет его
	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:  "t_secrets_cross",
			Input: map[string]interface{}{"value": "x"},
			Steps: []Step{
				{ID: "net", Plugin: fxPlugins + "net_demo", OnError: "stop"},
			},
		},
	}
	_, warns := Validate(pf, NewEngine())
	joined := strings.Join(warns, "; ")
	if !strings.Contains(joined, "плагину нужен ключ NET_DEMO_KEY") {
		t.Fatalf("ожидалось кросс-предупреждение о незадекларированном ключе: %v", warns)
	}

	// пайплайн объявляет секреты, которых не использует ни один плагин
	pf2 := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:    "t_secrets_orphan",
			Input:   map[string]interface{}{"value": "x"},
			Secrets: []string{"ORPHAN_KEY"},
			Steps: []Step{
				{ID: "echo", Plugin: fxPlugins + "echo_ok", OnError: "stop"},
			},
		},
	}
	_, warns2 := Validate(pf2, NewEngine())
	joined2 := strings.Join(warns2, "; ")
	if !strings.Contains(joined2, "ни один плагин не заявляет её") {
		t.Fatalf("ожидалось предупреждение об осиротевшем секрете: %v", warns2)
	}

	// всё согласовано → нет предупреждений про NET_DEMO_KEY
	pf3 := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:    "t_secrets_ok",
			Input:   map[string]interface{}{"value": "x"},
			Secrets: []string{"NET_DEMO_KEY"},
			Steps: []Step{
				{ID: "net", Plugin: fxPlugins + "net_demo", OnError: "stop"},
			},
		},
	}
	_, warns3 := Validate(pf3, NewEngine())
	for _, w := range warns3 {
		if strings.Contains(w, "NET_DEMO_KEY") && !strings.Contains(w, "не задана") {
			t.Fatalf("не должно быть кросс-предупреждений про NET_DEMO_KEY: %v", warns3)
		}
	}
}
