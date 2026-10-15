package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// labelWidth is where editor inputs start: the "› Title       " column.
const labelWidth = 14

func (model *Model) updateMouse(mouse tea.MouseMsg) tea.Cmd {
	if mouse.Action != tea.MouseActionPress {
		return nil
	}
	wheel := 0
	switch mouse.Button {
	case tea.MouseButtonWheelUp:
		wheel = -1
	case tea.MouseButtonWheelDown:
		wheel = 1
	case tea.MouseButtonLeft:
	default:
		return nil
	}
	model.message = ""
	switch {
	case model.mode == pickMode:
		return model.pickerMouse(mouse.Y, wheel)
	case model.mode == helpMode && wheel != 0:
		model.helpOffset = max(model.helpOffset+wheel, 0)
		return nil
	case wheel != 0 && model.mode != listMode && model.mode != searchMode:
		return nil
	case model.mode == helpMode:
		model.mode = listMode
		return nil
	case model.mode == confirmMode:
		return nil
	case model.mode == conflictMode:
		return model.conflictMouse(mouse.X, mouse.Y)
	}

	wide := model.previewWidth() > 0
	paneX := mouse.X - model.listWidth() - 2 // separator column and the pane's leading space
	switch {
	case model.mode == editMode && !wide:
		return model.editorMouse(mouse.X, mouse.Y, wheel)
	case model.mode == editMode && paneX >= 0:
		return model.editorMouse(paneX, mouse.Y, wheel)
	case wide && paneX >= 0:
		// clicking the preview starts editing the selected task at the clicked field
		selected, ok := model.selected()
		if !ok || wheel != 0 {
			return nil
		}
		model.leaveSearch()
		fieldLines := model.preview.fieldLines
		model.openEditor(selected, false)
		model.editor.fieldLines = fieldLines
		return model.editorMouse(paneX, mouse.Y, 0)
	}
	if wheel != 0 {
		model.move(wheel)
		return nil
	}
	// a click in the list leaves the editor beside it, saving like esc
	if model.mode == editMode && !model.closeEditor() {
		return nil
	}
	model.leaveSearch()
	return model.listMouse(mouse.X, mouse.Y)
}

func (model *Model) leaveSearch() {
	if model.mode == searchMode {
		model.mode = listMode
		model.search.Blur()
	}
}

func (model *Model) listMouse(x, y int) tea.Cmd {
	index := model.offset + y - 1
	if y >= 1 && index < len(model.rows) && model.rows[index].note != "" {
		index--
	}
	if y < 1 || y > model.bodyHeight() || index >= len(model.rows) || !model.rows[index].isTask() {
		return nil
	}
	wasSelected := index == model.cursor
	model.cursor = index
	model.scroll()
	boxStart := 2 + 4*model.rows[index].depth
	switch {
	case x >= boxStart && x < boxStart+3:
		model.toggleCompleted(model.rows[index].task)
	case wasSelected:
		model.open(model.rows[index].task)
	}
	return nil
}

func (model *Model) editorMouse(x, y, wheel int) tea.Cmd {
	form := &model.editor
	if wheel != 0 {
		return nil
	}
	for field := range fieldCount {
		line := form.fieldLines[field]
		inDescription := field == fieldDescription && y > line && y <= line+form.description.Height()
		if y != line && !inDescription {
			continue
		}
		if field == form.focus && field == fieldPriority {
			form.cycle(1)
			return nil
		}
		command := form.setFocus(field)
		if field != fieldDescription && field != fieldPriority {
			form.inputs[field].SetCursor(x - labelWidth)
		}
		return command
	}
	return nil
}

func (model *Model) pickerMouse(y, wheel int) tea.Cmd {
	if wheel != 0 {
		model.picker.cursor = max(min(model.picker.cursor+wheel, len(model.picker.items)-1), 0)
		return nil
	}
	index := model.pickerStart() + y - 2
	if y < 2 || index >= len(model.picker.items) || index >= model.pickerStart()+model.pickerHeight() {
		return nil
	}
	if index == model.picker.cursor {
		model.choosePicker()
	} else {
		model.picker.cursor = index
	}
	return nil
}

func (model *Model) conflictMouse(x, y int) tea.Cmd {
	if y != strings.Count(model.viewConflict(), "\n") {
		return nil
	}
	for _, option := range []string{"[l] keep local", "[r] keep remote", "[v] view differences", "[c] cancel"} {
		start := strings.Index(conflictKeys, option)
		if x >= start && x < start+len(option) {
			return model.updateConflict(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(option[1])}})
		}
	}
	return nil
}
