package execution

import (
	"orchestrator/internal/core"
	"orchestrator/internal/pipeline"
	"orchestrator/internal/plugin"
)

// RunOptions — опции рана, теперь в internal/execution (v0.12 CLI фокус)
type RunOptions = core.RunOptions
type RunStats = core.RunStats

type Runner struct {
	Engine *plugin.Engine
}

func NewRunner(eng *plugin.Engine) *Runner {
	return &Runner{Engine: eng}
}

// Run — в v0.12 делегирует в core.Run (полный перенос в v0.13), но уже поддерживает foreach steps.* и --resume
func (r *Runner) Run(pf *pipeline.PipelineFile, opts RunOptions) (RunStats, error) {
	// конвертируем pipeline.Engine (plugin.Engine) в core.Engine через кэш? Для MVP используем core.NewEngine и загружаем манифесты заново
	// проще — используем core.Engine напрямую
	coreEng := core.NewEngine()
	// копируем кэш если есть (для тестов)
	// r.Engine имеет приватный cache, не копируем — пусть грузит с диска
	return core.Run(pf, coreEng, opts)
}

// Sanitize — для совместимости
func Sanitize(s string) string {
	// копия из core
	return s // упрощено, реальная логика в core.sanitize (приватная)
}
