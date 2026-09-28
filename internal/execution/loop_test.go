package execution

import (
	"testing"

	"wedra/internal/pipeline"
)

func TestRunStepLoopMaxIterations(t *testing.T) {
	st := &pipeline.Step{
		ID:            "test_loop",
		Loop:          "parser",
		LoopCondition: "steps.parser.continue",
		MaxIterations: 3,
	}
	if st.MaxIterations != 3 {
		t.Error("MaxIterations не установлен")
	}
}

func TestRunStepLoopDefaultMaxIterations(t *testing.T) {
	st := &pipeline.Step{
		ID:   "test_loop",
		Loop: "parser",
	}
	if st.MaxIterations != 0 {
		t.Error("MaxIterations должен быть 0 по умолчанию")
	}
}

func TestMaxLoopIterationsConstant(t *testing.T) {
	if pipeline.MaxLoopIterations != 100 {
		t.Error("MaxLoopIterations должен быть 100")
	}
}
