package tui

import (
	_ "embed"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

//go:embed help.txt
var helpText string

// SetAccent replaces the default accent (titles, help headings and keys); empty keeps the default.
func SetAccent(color string) {
	if color == "" {
		return
	}
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	titleStyle = accentStyle.Bold(true)
}

// viewHelp scrolls when the help is taller than the terminal.
func (model *Model) viewHelp() string {
	lines := strings.Split("\n"+Markup(helpText), "\n")
	height := max(model.height, 1)
	model.helpOffset = max(min(model.helpOffset, len(lines)-height), 0)
	return strings.Join(lines[model.helpOffset:min(model.helpOffset+height, len(lines))], "\n")
}

// Markup renders a help file: a line starting with "# " is a heading, text in `backticks` is a key.
// Both are drawn in the accent color; the backticks themselves are dropped.
func Markup(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for index, line := range lines {
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		if heading, found := strings.CutPrefix(line[len(indent):], "# "); found {
			lines[index] = indent + titleStyle.Render(heading)
			continue
		}
		parts := strings.Split(line, "`")
		for part := 1; part < len(parts); part += 2 {
			parts[part] = accentStyle.Render(parts[part])
		}
		lines[index] = strings.Join(parts, "")
	}
	return strings.Join(lines, "\n")
}
