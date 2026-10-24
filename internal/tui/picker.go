package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/puhi8/tasks/internal/syncer"
	"github.com/puhi8/tasks/internal/task"
)

type pickerItem struct {
	label string
	value string
	style lipgloss.Style
}

type picker struct {
	title  string
	hint   string
	items  []pickerItem
	cursor int
	choose func(value string)
	onKey  func(key string, item *pickerItem) // optional, gets keys the picker does not use itself
}

func (model *Model) openPicker(title string, items []pickerItem, choose func(value string)) {
	model.picker = picker{title: title, items: items, choose: choose}
	model.mode = pickMode
}

func (model *Model) updatePicker(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "j", "down":
		model.picker.cursor = min(model.picker.cursor+1, len(model.picker.items)-1)
	case "k", "up":
		model.picker.cursor = max(model.picker.cursor-1, 0)
	case "enter":
		model.choosePicker()
	case "esc", "q":
		model.mode = listMode
	default:
		if model.picker.onKey != nil && len(model.picker.items) > 0 {
			model.picker.onKey(key.String(), &model.picker.items[model.picker.cursor])
		}
	}
	return nil
}

func (model *Model) choosePicker() {
	model.mode = listMode
	if len(model.picker.items) > 0 {
		model.picker.choose(model.picker.items[model.picker.cursor].value)
	}
}

func (model *Model) pickerHeight() int { return max(model.height-4, 1) }

func (model *Model) pickerStart() int {
	return max(min(model.picker.cursor-model.pickerHeight()/2, len(model.picker.items)-model.pickerHeight()), 0)
}

func (model *Model) viewPicker() string {
	lines := []string{titleStyle.Render(" " + model.picker.title), ""}
	height, start := model.pickerHeight(), model.pickerStart()
	for index := start; index < len(model.picker.items) && index < start+height; index++ {
		item := model.picker.items[index]
		style, prefix := item.style, "  "
		if index == model.picker.cursor {
			style, prefix = style.Inherit(selectedStyle), "› "
		}
		lines = append(lines, style.Render(ansi.Truncate(prefix+item.label, model.width-1, "…")))
	}
	if len(model.picker.items) == 0 {
		lines = append(lines, dimStyle.Render("  nothing to choose"))
	}
	return strings.Join(lines, "\n") + "\n\n" + dimStyle.Render(" j/k move · enter choose · esc cancel"+model.picker.hint)
}

func (model *Model) calendarItems(except string) []pickerItem {
	var items []pickerItem
	for _, calendar := range model.calendars {
		if calendar.Path != except {
			items = append(items, pickerItem{label: calendar.Name, value: calendar.Path})
		}
	}
	return items
}

func (model *Model) openListPicker() {
	items := append([]pickerItem{{label: "All lists", value: ""}}, model.calendarItems("")...)
	model.openPicker("Switch list", items, func(value string) {
		model.calendarPath = value
		model.refresh()
		model.move(-len(model.rows))
	})
	model.picker.hint = " · d delete list"
	model.picker.onKey = func(key string, item *pickerItem) {
		if key == "d" && item.value != "" {
			model.deleteList(item.value)
		}
	}
}

func (model *Model) openFilterPicker() {
	options := []string{"status:open", "status:completed", "due:overdue", "due:today", "due:soon", "due:none",
		"priority:high", "priority:medium", "priority:low", "priority:none"}
	items := []pickerItem{{label: "clear all filters and search", value: ""}}
	for _, option := range options {
		items = append(items, pickerItem{label: option, value: option})
	}
	model.openPicker("Filter", items, func(value string) {
		if value == "" {
			model.query = ""
		} else {
			key, content, _ := strings.Cut(value, ":")
			model.query = setQueryToken(model.query, key+":", content)
		}
		model.refresh()
	})
}

func (model *Model) tagCounts(calendarPath string) ([]string, map[string]int) {
	counts := map[string]int{}
	for _, todo := range model.calendarTasks(calendarPath) {
		for _, category := range todo.Categories {
			counts[category]++
		}
	}
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
	return tags, counts
}

func (model *Model) openTagPicker() {
	tags, counts := model.tagCounts(model.calendarPath)
	items := []pickerItem{{label: "any tag", value: ""}}
	names := map[string]string{}
	for _, tag := range tags {
		names[tagToken(tag)] = tag
		items = append(items, pickerItem{label: fmt.Sprintf("#%s (%d)", tag, counts[tag]), value: tagToken(tag), style: model.tagStyle(tag)})
	}
	model.openPicker("Filter by tag", items, func(value string) {
		model.query = setQueryToken(setQueryToken(model.query, "tag:", ""), "#", value)
		model.refresh()
	})
	model.picker.hint = " · c change color · C automatic color · d delete tag"
	model.picker.onKey = func(key string, item *pickerItem) {
		tag := names[item.value]
		if tag == "" || (key != "c" && key != "C" && key != "d") {
			return
		}
		if key == "d" {
			model.deleteTag(tag)
			return
		}
		color := ""
		if key == "c" {
			color = tagPalette[(slices.Index(tagPalette, model.tagColors[strings.ToLower(tag)])+1)%len(tagPalette)]
		}
		if err := model.database.SetTagColor(tag, color); err != nil {
			model.message = err.Error()
			return
		}
		model.reload()
		item.style = model.tagStyle(tag)
	}
}

// tagPalette is what c cycles through; stored as hex so every client shows the same color.
var tagPalette = []string{"#e5484d", "#f76b15", "#ffc53d", "#46a758", "#12a594", "#0090ff", "#8e4ec6", "#d6409f", "#8b8d98"}

func (model *Model) openMovePicker(selected task.Task) {
	if model.hasConflict(task.Subtree(model.tasks, selected)) {
		return
	}
	model.openPicker("Move “"+selected.Summary+"” (and its subtasks) to", model.calendarItems(selected.CalendarPath), func(value string) {
		if err := syncer.Move(model.database, model.tasks, selected, value); err != nil {
			model.message = "move failed: " + err.Error()
			return
		}
		model.reload()
		model.message = "moved to " + model.calendarName(value)
	})
}

func (model *Model) openParentPicker(selected task.Task) {
	siblings := model.calendarTasks(selected.CalendarPath)
	items := []pickerItem{{label: "(no parent)", value: ""}}
	for _, candidate := range siblings {
		if candidate.UID != selected.UID && !candidate.Done() && !task.WouldCycle(siblings, selected.UID, candidate.UID) {
			items = append(items, pickerItem{label: candidate.Summary, value: candidate.UID})
		}
	}
	model.openPicker("Move “"+selected.Summary+"” under", items, func(value string) { model.setParent(selected, value) })
}
