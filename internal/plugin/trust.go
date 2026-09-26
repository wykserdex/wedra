package plugin

import (
	"context"

	"wedra/internal/pipeline"
)

// Политика доверия к коду плагина. Плагин приходит извне (community registry,
// локальная папка пользователя) и исполняется с правами пользователя: скрипт
// может прочитать любой файл, доступный процессу, и утащить env. Поэтому
// запуск внешнего кода должен быть осознанным решением ядра, а не полем,
// которые плагин пишет в своём манифесте.
//
// Политика живёт в context, чтобы не менять сигнатуры всех Exec-вариантов:
// ран (runner) ставит политику один раз, все шаги наследуют её через ctx.

// TrustPolicy — решение ядра о том, чему можно исполняться.
type TrustPolicy struct {
	// DenyUntrusted — все плагины считаются внешними, включая те, что не
	// объявили sandbox: untrusted. Включается флагом
	// --deny-untrusted-plugins: полезно для CI и рандов с community-плагинами.
	DenyUntrusted bool
	// AllowUntrusted — разрешить запуск внешнего кода без изоляции. Требует
	// явного вызова AllowUntrustedPlugins; в CLI не выставляется.
	AllowUntrusted bool
}

type trustPolicyKey struct{}

// WithTrustPolicy возвращает ctx с политикой доверия.
func WithTrustPolicy(ctx context.Context, p TrustPolicy) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, trustPolicyKey{}, p)
}

// TrustPolicyFrom возвращает политику из ctx (нулевая — доверие как раньше).
func TrustPolicyFrom(ctx context.Context) TrustPolicy {
	if ctx == nil {
		return TrustPolicy{}
	}
	p, _ := ctx.Value(trustPolicyKey{}).(TrustPolicy)
	return p
}

// AllowUntrustedPlugins — вызов в trusted-коде (agent track, тесты), который
// осознанно берёт на себя риск запуска внешнего кода без изоляции.
func AllowUntrustedPlugins(ctx context.Context) context.Context {
	return WithTrustPolicy(ctx, TrustPolicy{AllowUntrusted: true})
}

// untrustedCodeError — описание отказа, возвращаемое как платформенная ошибка
// протокола (ErrCode "sandbox_unavailable"): ран падает, ничего не запускается.
func untrustedCodeError(m *pipeline.Manifest, reason string) *ExecResult {
	return &ExecResult{
		Platform: true,
		ErrCode:  "sandbox_unavailable",
		ErrMsg: "плагин " + m.ID + " помечен как внешний код (sandbox: untrusted), " +
			"но изоляция недоступна: " + reason + " — запуск невозможен",
		ExitCode: 2,
	}
}

// enforceTrust — fail-closed проверка перед запуском процесса. Возвращает
// готовый ExecResult с ошибкой, если код запускать нельзя.
func enforceTrust(m *pipeline.Manifest, policy TrustPolicy) *ExecResult {
	if m == nil {
		return nil
	}
	switch {
	case policy.DenyUntrusted:
		return untrustedCodeError(m, "ядро запущено с --deny-untrusted-plugins")
	case m.Untrusted() && !policy.AllowUntrusted:
		return untrustedCodeError(m,
			"sandbox backend в этой сборке не реализован (os-sandbox: bwrap/sandbox-exec/AppContainer)")
	}
	return nil
}
