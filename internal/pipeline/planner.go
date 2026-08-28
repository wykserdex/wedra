package pipeline

// Planner — DAG + dry-run план (M6)
// Сейчас — заглушка, основная логика в Validate (проверка порядка, foreach)
// В M6 планируется:
// - построение графа зависимостей (steps.* → steps.*)
// - детект циклов
// - топологическая сортировка
// - dry-run: какие плагины будут вызваны, какие данные пойдут по портам
type Plan struct {
	PipelineFile *PipelineFile
	Steps        []Step
	Warnings     []string
	Errors       []string
}

func PlanPipeline(pf *PipelineFile, eng Engine) (*Plan, error) {
	errs, warns := Validate(pf, eng)
	return &Plan{
		PipelineFile: pf,
		Steps:        pf.Pipeline.Steps,
		Warnings:     warns,
		Errors:       errs,
	}, nil
}
