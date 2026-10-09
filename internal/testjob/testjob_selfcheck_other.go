//go:build !windows

package testjob

// SelfTest — no-op вне Windows: проверять нечего, job'а нет. Возвращает nil,
// чтобы вызывающий TestMain не различал платформы.
func SelfTest() error { return nil }

// Handle всегда 0 вне Windows.
func Handle() uintptr { return 0 }
