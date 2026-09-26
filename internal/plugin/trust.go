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
	// AllowUntrusted — явное согласие запускать внешний код. Плагин всё равно
	// уходит в изолятор; если изолятора на хосте нет, запуск падает
	// (fail-closed), а не выполняется без песочницы.
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

// AllowUntrustedPlugins — вызов в trusted-коде, который осознанно разрешает
// запуск внешнего кода (он всё равно уходит в изолятор).
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

// enforceTrust — fail-closed проверка ДО запуска: без согласия ядра внешний код
// не запускается. Само наличие песочницы проверяется в execPluginEnv: там, где
// изолятора нет, процесс не создаётся (ErrSandboxUnsupported).
func enforceTrust(m *pipeline.Manifest, policy TrustPolicy) *ExecResult {
	if m == nil {
		return nil
	}
	switch {
	case policy.DenyUntrusted:
		return untrustedCodeError(m, "ядро запущено с --deny-untrusted-plugins")
	case m.Untrusted() && !policy.AllowUntrusted:
		return untrustedCodeError(m,
			"нет согласия на запуск внешнего кода (нужен флаг --allow-untrusted-plugins)")
	}
	return nil
}
