// Command um is a keyboard-driven terminal music player.
// Usage: um [music-dir]   (defaults to the current directory)
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ohhmkar/um/internal/tui"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "um: %q is not a directory\n", dir)
		os.Exit(1)
	}

	p := tea.NewProgram(tui.NewModel(dir), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "um:", err)
		os.Exit(1)
	}
}
