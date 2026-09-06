// Package web — GUI-ассеты, вшитые в бинарник (v0.7: go:embed).
//
// Release-бинарники (GitHub Release) теперь полностью автономны: GUI
// (консоль + редактор) работает без репо, без web/static рядом, на любой
// ОС. Из checkout (dev-режим) server отдаёт frontend с диска — горячая
// правка JS без пересборки.
package web

import "embed"

// FS — web/static как FS: пути "static/index.html", "static/editor/…".
//
//go:embed static
var FS embed.FS
