package pipeline

// Гейт перед опасным шагом одобряет только тогда, когда он действительно
// выполнится и его отказ остановит ран. Позиция в списке — необходимое, но
// не достаточное условие (аудит, повторная проверка H2, N2).

import (
	"strings"
	"testing"
)

func gateStep(mod func(*Step)) Step {
	g := Step{ID: "review", Plugin: "core/human_gate", Actions: []string{"accept", "reject"}}
	if mod != nil {
		mod(&g)
	}
	return g
}

func TestEvaluateGateApprovalIneffectiveGates(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{"net": netPlugin()})
	danger := Step{ID: "net", Plugin: "net"}

	cases := []struct {
		name       string
		mod        func(*Step)
		wantReason string // подстрока причины; пусто — гейт обязан засчитаться
	}{
		{"обычный гейт", nil, ""},
		{"on_reject: stop", func(s *Step) { s.OnReject = "stop" }, ""},
		{"on_error: stop", func(s *Step) { s.OnError = "stop" }, ""},
		{"on_reject: continue", func(s *Step) { s.OnReject = "continue" }, "on_reject: continue"},
		{"on_error: skip", func(s *Step) { s.OnError = "skip" }, "on_error: skip"},
		{"when по пути", func(s *Step) { s.When = When{Path: "input.t", Op: "eq", Value: "never"} }, "when"},
		{"when только со значением", func(s *Step) { s.When = When{Value: true} }, "when"},
	}
	for _, tc := range cases {
		pf := gatePipeline(gateStep(tc.mod), danger)
		req := EvaluateGateApproval(pf, eng)
		if tc.wantReason == "" {
			if req.Required {
				t.Errorf("%s: гейт должен засчитываться, а получен отказ: %+v", tc.name, req)
			}
			continue
		}
		if !req.Required {
			t.Errorf("%s: гейт НЕ гарантирует одобрение, но опасный шаг пропущен", tc.name)
			continue
		}
		if req.GateID != "review" || !strings.Contains(req.GateIneffective, tc.wantReason) {
			t.Errorf("%s: причина %q (гейт %q), ждали подстроку %q", tc.name, req.GateIneffective, req.GateID, tc.wantReason)
		}
		if req.GateAfter {
			t.Errorf("%s: гейт стоит ПЕРЕД шагом, GateAfter должен быть false", tc.name)
		}
	}
}

// Несколько гейтов: достаточно одного действующего перед шагом.
func TestEvaluateGateApprovalOneEffectiveGateIsEnough(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{"net": netPlugin()})
	bad := gateStep(func(s *Step) { s.ID = "bad"; s.OnReject = "continue" })
	good := gateStep(func(s *Step) { s.ID = "good" })
	if EvaluateGateApproval(gatePipeline(bad, good, Step{ID: "net", Plugin: "net"}), eng).Required {
		t.Error("действующий гейт good стоит перед шагом — одобрение должно засчитаться")
	}
	if !EvaluateGateApproval(gatePipeline(bad, Step{ID: "net", Plugin: "net"}, good), eng).Required {
		t.Error("действующий гейт ПОСЛЕ шага не защищает")
	}
}

// Каждый опасный шаг проверяется отдельно: действующий гейт перед первым шагом
// не должен «покрывать» шаг, для которого он не сработает.
func TestEvaluateGateApprovalChecksEveryDangerousStep(t *testing.T) {
	eng := rightsEngine(map[string]*Manifest{"net": netPlugin()})
	pf := gatePipeline(
		gateStep(func(s *Step) { s.AfterForeach = true }),
		Step{ID: "per_item", Plugin: "net"},
	)
	pf.Pipeline.Foreach = "input.items"
	req := EvaluateGateApproval(pf, eng)
	if !req.Required || req.Step != "per_item" || !strings.Contains(req.GateIneffective, "after_foreach") {
		t.Errorf("гейт after_foreach не защищает шаг по элементам: %+v", req)
	}

	// Без foreach-пайплайна after_foreach ничего не меняет.
	pf.Pipeline.Foreach = ""
	if EvaluateGateApproval(pf, eng).Required {
		t.Error("без pipeline.foreach after_foreach не сдвигает порядок шагов")
	}

	// Опасный шаг тоже after_foreach: порядок сохраняется, гейт действует.
	pf.Pipeline.Foreach = "input.items"
	pf.Pipeline.Steps[1].AfterForeach = true
	if EvaluateGateApproval(pf, eng).Required {
		t.Error("оба шага after_foreach и гейт выше — одобрение должно засчитаться")
	}
}

func TestGateIneffectiveReasonNilGate(t *testing.T) {
	if GateIneffectiveReason(nil, nil, nil) == "" {
		t.Error("отсутствующий гейт не может считаться действующим")
	}
}
