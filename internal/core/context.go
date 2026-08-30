package core

import ctxpkg "wedra/internal/context"

type Ctx = ctxpkg.Ctx

func NewCtx(input map[string]interface{}) *Ctx {
	return ctxpkg.NewCtx(input)
}
