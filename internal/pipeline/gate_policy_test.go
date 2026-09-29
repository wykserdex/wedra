package pipeline

import "testing"

// Матрица «опасен ли шаг». Права берутся из permissions манифеста, поэтому
// тест проверяет ровно объявление — и отдельно то, что отсутствие объявления
// (nil-манифест, битый plugin.yaml) считается опасным, а не безопасным.
type capsCase struct {
	name     string
	manifest *Manifest
	want     Capabilities
}

func TestStepCapabilitiesMatrix(t *testing.T) {
	cases := []capsCase{
		{"пустой манифест", &Manifest{ID: "p"}, Capabilities{}},
		{"только чтение", &Manifest{ID: "p", Permissions: Permissions{Filesystem: "read"}}, Capabilities{}},
		{"сеть", &Manifest{ID: "p", Permissions: Permissions{
			Network: []NetworkPermission{{Host: "api.example.com", Port: 443}}}}, Capabilities{Network: true}},
		{"запись в рабочую папку", &Manifest{ID: "p", Permissions: Permissions{Filesystem: "workspace"}}, Capabilities{DiskWrite: true}},
		{"запись", &Manifest{ID: "p", Permissions: Permissions{Filesystem: "write"}}, Capabilities{DiskWrite: true}},
		{"чтение и запись", &Manifest{ID: "p", Permissions: Permissions{Filesystem: "readwrite"}}, Capabilities{DiskWrite: true}},
		{"секреты", &Manifest{ID: "p", Permissions: Permissions{Secrets: []string{"TOKEN"}}}, Capabilities{Secrets: true}},
		{"всё сразу", &Manifest{ID: "p", Permissions: Permissions{
			Network:    []NetworkPermission{{AnyHost: true}},
			Filesystem: "readwrite",
			Secrets:    []string{"TOKEN"}}}, Capabilities{Network: true, DiskWrite: true, Secrets: true}},
	}
	for _, tc := range cases {
		got := StepCapabilities(&Step{ID: "s", Plugin: "some/plugin"}, tc.manifest)
		if got != tc.want {
			t.Errorf("%s: capabilities = %+v, ждали %+v", tc.name, got, tc.want)
		}
		wantSome := tc.want != Capabilities{}
		if got.Dangerous() != wantSome {
			t.Errorf("%s: Dangerous() = %v при правах %+v", tc.name, got.Dangerous(), got)
		}
		if wantSome && got.Why() == "" {
			t.Errorf("%s: Why() пуст при непустых правах — сообщение агенту будет бесполезным", tc.name)
		}
	}
}

// Непрочитанный манифест — не «прав нет», а «прав не знаем». Отказ по умолчанию.
func TestStepCapabilitiesTreatsUnknownAsDangerous(t *testing.T) {
	got := StepCapabilities(&Step{ID: "s", Plugin: "some/plugin"}, nil)
	if !got.Dangerous() {
		t.Fatal("неизвестные права не должны считаться безопасными")
	}
	if got.Why() == "" {
		t.Error("Why() должен объяснять, что манифест не прочитан")
	}
}

// Встроенные модули — доверенный код ядра, прав у них нет. В том числе text_stats:
// он не гейт, и его наличие не должно засчитываться за одобрение человека.
func TestBuiltinStepsDeclareNoCapabilities(t *testing.T) {
	for _, ref := range []string{"core/human_gate", "core/text_stats", "core\\text_stats"} {
		got := StepCapabilities(&Step{ID: "s", Plugin: ref}, &Manifest{ID: ref})
		if got.Dangerous() {
			t.Errorf("%s: встроенный модуль не должен считаться опасным, получено %+v", ref, got)
		}
	}
}

func TestIsHumanGateDoesNotAcceptOtherBuiltins(t *testing.T) {
	if !IsHumanGate("core/human_gate") {
		t.Error("core/human_gate должен распознаваться как гейт")
	}
	for _, ref := range []string{"core/text_stats", "core/неизвестный", "some/plugin", ""} {
		if IsHumanGate(ref) {
			t.Errorf("%q не должен считаться гейтом человека", ref)
		}
	}
}

// rightsEngine — манифесты без диска. Отсутствующий ref даёт ошибку загрузки,
// как битый plugin.yaml. Переиспользует stubEngine пакета (issue_test.go).
func rightsEngine(mans map[string]*Manifest) *stubEngine {
	return &stubEngine{mans: mans}
}

func writeFSManifest(fs string) *Manifest {
	return &Manifest{ID: "p", Permissions: Permissions{Filesystem: fs}}
}

func netPlugin() *Manifest {
	return &Manifest{ID: "net", Permissions: Permissions{
		Network: []NetworkPermission{{AnyHost: true}}}}
}

func secretPlugin() *Manifest {
	return &Manifest{ID: "s", Permissions: Permissions{Secrets: []string{"TOKEN"}}}
}

func gatePipeline(steps ...Step) *PipelineFile {
	return &PipelineFile{FormatVersion: "0.2", Pipeline: Pipeline{Name: "p", Steps: steps}}
}

func TestEvaluateGateApprovalRequiresGateBeforeDangerousStep(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{
		"net":    netPlugin(),
		"fs":     writeFSManifest("workspace"),
		"secret": secretPlugin(),
		"ro1":    writeFSManifest("read"),
		"ro2":    &Manifest{ID: "ro2"},
	})
	cases := []struct {
		name string
		pf   *PipelineFile
		want bool
	}{
		{"опасный шаг без гейта", gatePipeline(Step{ID: "net", Plugin: "net"}), true},
		{"опасный шаг, гейт выше", gatePipeline(
			Step{ID: "review", Plugin: "core/human_gate"},
			Step{ID: "net", Plugin: "net"}), false},
		{"безопасные шаги без гейта", gatePipeline(
			Step{ID: "a", Plugin: "ro1"},
			Step{ID: "b", Plugin: "ro2"}), false},
		{"безопасный шаг ВМЕСТЕ с опасным, гейт между ними", gatePipeline(
			Step{ID: "count", Plugin: "ro1"},
			Step{ID: "review", Plugin: "core/human_gate"},
			Step{ID: "net", Plugin: "net"}), false},
		{"гейт после опасного шага", gatePipeline(
			Step{ID: "net", Plugin: "net"},
			Step{ID: "review", Plugin: "core/human_gate"}), true},
		{"core/text_stats гейтом не считается", gatePipeline(
			Step{ID: "stats", Plugin: "core/text_stats"},
			Step{ID: "net", Plugin: "net"}), true},
		{"непрочитанный манифест", gatePipeline(Step{ID: "битый", Plugin: "нет-такого"}), true},
		{"ни одного шага", gatePipeline(), false},
	}
	for _, tc := range cases {
		req := EvaluateGateApproval(tc.pf, eng)
		if req.Required != tc.want {
			t.Errorf("%s: Required = %v, ждали %v (шаг %q, права %q)",
				tc.name, req.Required, tc.want, req.Step, req.Why)
		}
	}
}

// Отказ должен называть конкретный шаг и конкретные права: агенту нужен точный
// адрес, что чинить, а не «какой-то гейт».
func TestEvaluateGateApprovalNamesStepAndCapabilities(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{"secret": secretPlugin()})
	req := EvaluateGateApproval(gatePipeline(
		Step{ID: "llm", Plugin: "secret"},
		Step{ID: "review", Plugin: "core/human_gate"}), eng)
	if !req.Required {
		t.Fatal("опасный шаг без гейта должен требовать одобрения")
	}
	if req.Step != "llm" || req.Plugin != "secret" {
		t.Errorf("отказ должен называть шаг llm/плагин secret, получено %q/%q", req.Step, req.Plugin)
	}
	if req.Why != "чтение секретов" {
		t.Errorf("Why = %q, ждали «чтение секретов»", req.Why)
	}
	if !req.GateAfter || req.GateID != "review" {
		t.Errorf("гейт после опасного шага должен быть назван: %+v", req)
	}
}

// Права на запись объявляет и filesystem: workspace — объявление означает
// доступ к рабочей папке, а не «только чтение».
func TestEvaluateGateApprovalTreatsWorkspaceAsDiskWrite(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{"ws": writeFSManifest("workspace"), "ro": writeFSManifest("read")})
	if EvaluateGateApproval(gatePipeline(Step{ID: "ro", Plugin: "ro"}), eng).Required {
		t.Error("filesystem: read — не запись, одобрение не требуется")
	}
	if !EvaluateGateApproval(gatePipeline(Step{ID: "ws", Plugin: "ws"}), eng).Required {
		t.Error("filesystem: workspace — право записи, одобрение требуется")
	}
}
