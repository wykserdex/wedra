//go:build !windows

package main

import "fmt"

// desktop (не Windows) — dev-путь: GUI в системном браузере, процесс живёт
// до Ctrl+C. Настоящее окно — на Windows (WebView2, desktop_windows.go).
func desktop(url string, debug bool, logf func(string, ...interface{})) {
	logf("окно WebView2 — только Windows; здесь GUI открыт в браузере")
	// ссылка с ключом сессии — в консоль (не в лог-файл): если браузер не открылся
	fmt.Println("  ссылка с ключом сессии (только для вас):", url)
	openBrowser(url)
	waitSignal()
}
