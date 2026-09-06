//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/jchv/go-webview2"
)

// desktop (Windows) — собственное окно WebView2. Чистый Go (без CGO):
// lib вшит через go-winloader (WebView2Loader.dll), нужен только рантайм
// WebView2 (встроен в Windows 10 1803+). Нет рантайма → браузер.
func desktop(url string, debug bool, logf func(string, ...interface{})) {
	dataPath := ""
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		dataPath = filepath.Join(dir, "WEDRA", "webview2")
	}

	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:    debug,
		DataPath: dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  "WEDRA",
			Width:  1280,
			Height: 860,
			Center: true,
		},
	})
	if wv == nil {
		logf("WebView2 не запустился (нет рантайма?) — открой %s в браузере", url)
		logf("рантайм: https://developer.microsoft.com/microsoft-edge/webview2/")
		openBrowser(url)
		waitSignal()
		return
	}
	wv.Navigate(url)
	wv.Run() // блокирует до закрытия окна
	wv.Destroy()
}
