package core

import ctxpkg "github.com/wykserdex/wedra/internal/runctx"

type Ctx = ctxpkg.Ctx

func NewCtx(input map[string]interface{}) *Ctx {
	return ctxpkg.NewCtx(input)
}
