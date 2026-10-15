package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/puhi8/tasks/internal/task"
)

const (
	fieldTitle = iota
	fieldPriority
	fieldStart
	fieldDue
	fieldTags
	fieldLocation
	fieldRepeat
	fieldDescription
	fieldCount
)

var (
	fieldLabels    = [fieldCount]string{"Title", "Priority", "Start", "Due", "Tags", "Location", "Repeat", "Description"}
	priorityValues = []int{0, 9, 5, 1} // iCalendar: 1 is the highest, as Tasks.org writes them
	priorityLabels = []string{"None", "Low", "Medium", "High"}
)

type editor struct {
	original    task.Task
	isNew       bool
	focus       int
	inputs      [fieldCount]textinput.Model
	description textarea.Model
	priority    int // -1 keeps a priority value that has no label (2-4, 6-8)
	errorText   string
	// discardArmed is set after esc failed to save: the next esc closes without saving
	discardArmed bool
	// fieldLines is the screen line of each field at the last render, for mouse clicks
	fieldLines [fieldCount]int
	tags       []string
}

var placeholders = map[int]string{
	fieldStart: "YYYY-MM-DD [HH:MM], today, tomorrow, +3", fieldDue: "YYYY-MM-DD [HH:MM], today, tomorrow, +3",
	fieldTags: "comma separated", fieldRepeat: "daily, weekly, monthly, yearly or an RRULE like FREQ=WEEKLY;BYDAY=MO,TH"}

// emptyLabels stand in for empty fields in the preview; the editor replaces them with the typing hints above.
var emptyLabels = map[int]string{
	fieldStart: "no start date", fieldDue: "no due date", fieldTags: "no tags", fieldLocation: "no location", fieldRepeat: "no repeat"}

// newEditor builds the form for a task with nothing focused; the preview pane shows it like that.
func (model *Model) newEditor(todo task.Task, isNew bool, width int) editor {
	form := editor{original: todo, isNew: isNew, focus: -1, priority: slices.Index(priorityValues, todo.Priority)}
	values := map[int]string{
		fieldTitle: todo.Summary, fieldStart: formatDateInput(todo.Start), fieldDue: formatDateInput(todo.Due),
		fieldTags: strings.Join(todo.Categories, ", "), fieldLocation: todo.Location, fieldRepeat: todo.Recurrence}
	for field, value := range values {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = emptyLabels[field]
		input.SetValue(value)
		input.Blur()
		form.inputs[field] = input
	}
	form.inputs[fieldTags].ShowSuggestions = true
	form.tags, _ = model.tagCounts("")
	form.description = textarea.New()
	form.description.ShowLineNumbers = false
	form.description.Prompt = "│ "
	form.description.CharLimit = 0
	form.description.Placeholder = "no description"
	form.description.SetValue(todo.Description)
	form.description.Blur()
	form.resize(width, model.height)
	return form
}

func (model *Model) openEditor(todo task.Task, isNew bool) {
	form := model.newEditor(todo, isNew, model.editorWidth())
	for field := range form.inputs {
		form.inputs[field].Placeholder = placeholders[field]
	}
	form.description.Placeholder = ""
	form.setFocus(fieldTitle)
	model.editor = form
	model.mode = editMode
}

// resize shrinks the description box on short terminals; the rest of the form takes 15 lines.
func (form *editor) resize(width, height int) {
	for field := range form.inputs {
		form.inputs[field].Width = max(width-16, 10)
	}
	form.description.SetWidth(max(width-4, 10))
	form.description.SetHeight(max(min(height-15, 6), 1))
}

func (form *editor) setFocus(field int) tea.Cmd {
	form.focus = (field + fieldCount) % fieldCount
	for index := range form.inputs {
		form.inputs[index].Blur()
	}
	form.description.Blur()
	switch form.focus {
	case fieldDescription:
		return form.description.Focus()
	case fieldPriority:
		return nil
	}
	return form.inputs[form.focus].Focus()
}

func (form *editor) update(message tea.Msg) tea.Cmd {
	var command tea.Cmd
	switch form.focus {
	case fieldDescription:
		form.description, command = form.description.Update(message)
	case fieldPriority:
	default:
		form.inputs[form.focus], command = form.inputs[form.focus].Update(message)
		if form.focus == fieldTags {
			form.refreshTagSuggestions()
		}
	}
	return command
}

func (model *Model) updateEditor(key tea.KeyMsg) tea.Cmd {
	form := &model.editor
	discard := form.discardArmed
	form.discardArmed = false
	// while a tag is being typed, tab accepts and ↑/↓ choose among the matching known tags
	if form.focus == fieldTags && len(form.inputs[fieldTags].MatchedSuggestions()) > 0 {
		switch key.String() {
		case "tab", "up", "down":
			return form.update(key)
		}
	}
	switch key.String() {
	case "esc":
		// only when saving failed does a second esc throw the changes away
		if discard {
			model.mode = listMode
		} else if !model.closeEditor() {
			form.errorText += " · esc again discards the changes"
			form.discardArmed = true
		}
		return nil
	case "ctrl+s":
		model.saveEditor()
		return nil
	case "tab":
		return form.setFocus(form.focus + 1)
	case "shift+tab":
		return form.setFocus(form.focus - 1)
	}
	choice := form.focus == fieldPriority
	switch {
	case form.focus == fieldDescription:
		return form.update(key)
	case key.String() == "enter" || key.String() == "down" || (choice && key.String() == "j"):
		return form.setFocus(form.focus + 1)
	case key.String() == "up" || (choice && key.String() == "k"):
		return form.setFocus(form.focus - 1)
	case !choice:
		return form.update(key)
	case key.String() == "left" || key.String() == "h":
		form.cycle(-1)
	case key.String() == "right" || key.String() == "l" || key.String() == " ":
		form.cycle(1)
	}
	return nil
}

func (form *editor) cycle(delta int) {
	form.priority = (max(form.priority, 0) + delta + len(priorityValues)) % len(priorityValues)
}

// closeEditor saves and closes the form; a new task still without a title is simply dropped.
func (model *Model) closeEditor() bool {
	if model.editor.isNew && strings.TrimSpace(model.editor.inputs[fieldTitle].Value()) == "" {
		model.mode = listMode
		return true
	}
	return model.saveEditor()
}

// saveEditor closes the form after saving it; on invalid input it stays open with the error.
func (model *Model) saveEditor() bool {
	form := &model.editor
	edited := form.original
	edited.Summary = strings.TrimSpace(form.inputs[fieldTitle].Value())
	edited.Description = form.description.Value()
	if form.priority >= 0 {
		edited.Priority = priorityValues[form.priority]
	}
	edited.Location = strings.TrimSpace(form.inputs[fieldLocation].Value())
	edited.Categories = parseTags(form.inputs[fieldTags].Value(), form.original.Categories)
	if repeat := strings.TrimSpace(form.inputs[fieldRepeat].Value()); repeat != form.original.Recurrence {
		edited.Recurrence = task.ExpandRule(repeat)
	}
	var err error
	if edited.Start, err = parseDateInput(form.inputs[fieldStart].Value(), form.original.Start); err != nil {
		form.errorText = "Start: " + err.Error()
		return false
	}
	if edited.Due, err = parseDateInput(form.inputs[fieldDue].Value(), form.original.Due); err != nil {
		form.errorText = "Due: " + err.Error()
		return false
	}
	if err := task.Validate(edited); err != nil {
		form.errorText = err.Error()
		return false
	}
	if !model.saveTask(edited) {
		form.errorText = model.message
		return false
	}
	model.mode = listMode
	return true
}

// renderEditor draws the form; unfocused (focus -1) it is the read-only preview.
func (model *Model) renderEditor(form *editor, width int) string {
	title := "Edit task"
	switch {
	case form.focus < 0:
		title = "Task"
	case form.isNew:
		title = "New task"
	}
	details := "list " + model.calendarName(form.original.CalendarPath)
	for _, todo := range model.tasks {
		if form.original.ParentUID != "" && todo.UID == form.original.ParentUID && todo.CalendarPath == form.original.CalendarPath {
			details += " · subtask of " + todo.Summary
		}
	}
	lines := []string{titleStyle.Render(" "+title) + dimStyle.Render("  "+details), ""}
	for field := range fieldCount {
		label := fmt.Sprintf("  %-12s", fieldLabels[field])
		if field == form.focus {
			label = titleStyle.Render(fmt.Sprintf("› %-12s", fieldLabels[field]))
		}
		form.fieldLines[field] = len(lines)
		switch field {
		case fieldDescription:
			form.fieldLines[field]++
			lines = append(lines, "", label, form.description.View())
		case fieldPriority:
			value, level := "Custom ("+strconv.Itoa(form.original.Priority)+")", form.original.Priority
			if form.priority >= 0 {
				value, level = priorityLabels[form.priority], priorityValues[form.priority]
			}
			if form.focus < 0 && level == 0 {
				lines = append(lines, label+dimStyle.Render("no priority"))
			} else {
				lines = append(lines, label+"‹ "+priorityStyle[task.PriorityLabel(level)].Render(value)+" ›")
			}
		default:
			value := form.inputs[field].View()
			// a text input cannot color parts of its text, so tags are drawn colored whenever the field is not being edited
			if tags := parseTags(form.inputs[fieldTags].Value(), nil); field == fieldTags && form.focus != fieldTags && len(tags) > 0 {
				colored := make([]string, len(tags))
				for index, tag := range tags {
					colored[index] = model.tagStyle(tag).Render(tag)
				}
				value = strings.Join(colored, ", ")
			}
			lines = append(lines, label+value)
			if field == fieldTags && form.focus == fieldTags {
				if hints := model.tagHints(width); hints != "" {
					lines = append(lines, hints)
				}
			}
		}
	}
	if form.focus < 0 {
		lines = append(lines, "", dimStyle.Render(" enter edit"))
	} else {
		lines = append(lines, "", errorStyle.Render(" "+form.errorText),
			dimStyle.Render(" tab/shift+tab move · ←/→ change choice · esc save & close"))
	}
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], width, "…")
	}
	return strings.Join(lines, "\n")
}
