package gate

import (
	"orchestrator/internal/context"
	"orchestrator/internal/pipeline"
)

// Service — логика human_gate, вынесена из core/gate.go
// Сейчас — shim, основная логика пока в core

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Materialize(form []pipeline.FormField, ctx *context.Ctx) map[string]interface{} {
	// копия логики из core/gate.go — materialize
	// для MVP — заглушка, реальная логика в core
	return map[string]interface{}{}
}
