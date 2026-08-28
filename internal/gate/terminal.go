package gate

import (
	"bufio"
	"fmt"
	"os"
)

// Terminal — UI для human_gate (edit/accept/reject), вынесено из core/gate.go
type Terminal struct {
	Yes bool // auto-accept для CI
}

func NewTerminal(yes bool) *Terminal { return &Terminal{Yes: yes} }

func (t *Terminal) Prompt(prompt string) string {
	if t.Yes {
		return ""
	}
	fmt.Print(prompt)
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}

func (t *Terminal) Confirm(msg string) bool {
	if t.Yes {
		return true
	}
	fmt.Printf("%s [y/N]: ", msg)
	var ans string
	fmt.Scanln(&ans)
	return ans == "y" || ans == "Y"
}
