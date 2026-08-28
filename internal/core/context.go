package core

import ctxpkg "orchestrator/internal/context"

// Ctx — shared context, теперь живёт в internal/context, здесь алиас для совместимости
type Ctx = ctxpkg.Ctx

func NewCtx(input map[string]interface{}) *Ctx {
	return ctxpkg.NewCtx(input)
}
