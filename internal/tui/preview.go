package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const previewMinWidth = 100

func (model *Model) previewWidth() int {
	if model.width < previewMinWidth {
		return 0
	}
	return min(model.width*2/5, 72)
}

func (model *Model) listWidth() int {
	if preview := model.previewWidth(); preview > 0 {
		return model.width - preview - 1
	}
	return model.width
}

// editorWidth is the pane content width on wide terminals (the pane starts with a space), the whole screen otherwise.
func (model *Model) editorWidth() int {
	if preview := model.previewWidth(); preview > 0 {
		return preview - 1
	}
	return model.width
}

// previewPane renders the selected task in the editor layout without focus; rebuilt only when the task or width changes.
func (model *Model) previewPane() string {
	selected, ok := model.selected()
	if !ok {
		return dimStyle.Render(" nothing selected")
	}
	key := fmt.Sprint(model.editorWidth(), model.height) + "\x00" + selected.Path + "\x00" + selected.RawICalendar
	if key != model.previewKey {
		model.preview = model.newEditor(selected, false, model.editorWidth())
		model.previewKey = key
	}
	return model.renderEditor(&model.preview, model.editorWidth())
}

func (model *Model) split(pane string) string {
	width := model.previewWidth()
	lines := strings.Split(pane, "\n")
	for len(lines) < model.height {
		lines = append(lines, "")
	}
	lines = lines[:model.height]
	separator := make([]string, model.height)
	for index := range lines {
		lines[index] = " " + ansi.Truncate(lines[index], width-1, "…")
		separator[index] = dimStyle.Render("│")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, model.viewList(), strings.Join(separator, "\n"), strings.Join(lines, "\n"))
}
