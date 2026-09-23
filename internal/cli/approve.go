package cli

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
)

// isTTY — интерактивный терминал? Shell-инструменты агентов обычно без TTY.
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// shortCode — короткий код для подтверждения (4 цифры).
func shortCode() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000"
	}
	n := int(b[0])<<8 | int(b[1])
	return fmt.Sprintf("%04d", n%10000)
}

// RunApprove — wedra approve <run> <step> [--server=http://127.0.0.1:8765].
// Запасной вариант без браузера: только интерактивный TTY + короткий код.
// Shell-инструменты агентов обычно без TTY — само-аппрув через них закрыт.
func RunApprove(args []string) {
	var pos []string
	server := "http://127.0.0.1:8765"
	for _, a := range args {
		if strings.HasPrefix(a, "--server=") {
			server = strings.TrimPrefix(a, "--server=")
		} else if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
		}
	}
	if len(pos) < 2 {
		fmt.Println("нужно: wedra approve <run_id> <step_id> [--server=http://127.0.0.1:8765]")
		fmt.Println("одобрение — только человеком в интерактивном терминале")
		os.Exit(2)
	}
	runID, stepID := pos[0], pos[1]
	if !isTTY() {
		fmt.Println("отказ: approve работает только в интерактивном TTY (stdin не терминал)")
		fmt.Println("попросите пользователя одобрить шаг", stepID, "в окне wedra gui")
		os.Exit(1)
	}
	code := shortCode()
	fmt.Printf("подтвердите одобрение run %s шаг %s\n", runID, stepID)
	fmt.Printf("введите код %s: ", code)
	var input string
	_, _ = fmt.Scanln(&input)
	if strings.TrimSpace(input) != code {
		fmt.Println("код не совпал — отказ")
		os.Exit(1)
	}
	fmt.Printf("код принят. Отправьте решение человеком через GUI (%s) или API с сессией:\n", server)
	fmt.Printf("  POST %s/api/runs/%s/gate  {\"action\":\"accept\"}  (cookie сессии из терминала wedra gui)\n", server, runID)
}
