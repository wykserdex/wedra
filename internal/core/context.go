package core

import ctxpkg "orchestrator/internal/context"

type Ctx = ctxpkg.Ctx

func NewCtx(input map[string]interface{}) *Ctx {
	return ctxpkg.NewCtx(input)
}
