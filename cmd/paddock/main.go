package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/neyham/herdr-paddock/internal/ui"
)

func main() {
	p := tea.NewProgram(ui.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "paddock: %v\n", err)
		os.Exit(1)
	}
}
