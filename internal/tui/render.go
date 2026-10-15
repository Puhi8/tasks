package tui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/task"
)

func (model *Model) viewList() string {
	title := "All lists"
	if model.calendarPath != "" {
		title = model.calendarName(model.calendarPath)
	}
	var flags []string
	if model.query != "" {
		flags = append(flags, "search: "+model.query)
	}
	if model.showCompleted {
		flags = append(flags, "incl. completed")
	}
	if model.manualSort {
		flags = append(flags, "manual order")
	}
	header := titleStyle.Render(" "+title) + dimStyle.Render("  "+strings.Join(flags, " · "))

	var lines []string
	height := model.bodyHeight()
	for index := model.offset; index < len(model.rows) && index < model.offset+height; index++ {
		selected := index == model.cursor || (model.rows[index].note != "" && index-1 == model.cursor)
		lines = append(lines, model.renderRow(model.rows[index], selected))
	}
	if len(model.rows) == 0 {
		lines = append(lines, dimStyle.Render(model.emptyHint()))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	footer, bottom := model.footer(), model.message
	if model.mode == searchMode {
		bottom = model.search.View()
		// tag suggestions take the footer's place just above the search box while a # word is typed
		if suggestions := model.suggestionLine(); suggestions != "" {
			footer = suggestions
		}
	}
	return header + "\n" + strings.Join(lines, "\n") + "\n" + footer + "\n" + ansi.Truncate(bottom, model.listWidth(), "…")
}

func (model *Model) emptyHint() string {
	switch {
	case len(model.calendars) == 0:
		return "  no lists yet, press s to sync"
	case model.query != "":
		return "  nothing matches, esc clears the search"
	case len(model.calendarTasks(model.calendarPath)) > 0:
		return "  all done, h shows completed tasks"
	}
	return "  no tasks, press a to add one"
}

func (model *Model) footer() string {
	open, completed, conflicts := 0, 0, 0
	for _, todo := range model.calendarTasks(model.calendarPath) {
		if todo.Done() {
			completed++
		} else {
			open++
		}
		if todo.Conflict {
			conflicts++
		}
	}
	left := fmt.Sprintf(" %d open  %d completed", open, completed)
	if model.pending > 0 {
		left += fmt.Sprintf("  %d pending", model.pending)
	}
	if conflicts > 0 {
		left += errorStyle.Render(fmt.Sprintf("  %d conflict(s)", conflicts))
	}
	right := model.syncStatus
	if model.syncing {
		right = "syncing…"
	}
	right += "  ? help "
	gap := max(model.listWidth()-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return dimStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// tagColors leaves out red, which already means high priority and overdue.
var tagColors = []lipgloss.Color{"2", "3", "4", "5", "6", "10", "13", "14"}

func (model *Model) tagStyle(tag string) lipgloss.Style {
	if color, ok := model.tagColors[strings.ToLower(tag)]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	}
	hash := fnv.New32a()
	hash.Write([]byte(strings.ToLower(tag)))
	return lipgloss.NewStyle().Foreground(tagColors[hash.Sum32()%uint32(len(tagColors))])
}

func dueStyle(todo task.Task, now time.Time) lipgloss.Style {
	switch {
	case todo.Done():
		return dimStyle
	case overdue(todo, now):
		return errorStyle
	case daysBetween(now, todo.Due.Time) <= 1:
		return warningStyle
	}
	return lipgloss.NewStyle()
}

func (model *Model) renderRow(item row, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = selectedStyle
	}
	// every segment carries the row background so the selected row keeps its colors
	paint := func(style lipgloss.Style, text string) string { return style.Inherit(base).Render(text) }
	if item.header != "" {
		return paint(titleStyle, "── "+item.header)
	}
	if item.note != "" {
		// 6 columns are the marker and the checkbox, so the note starts under the title
		note := ansi.Truncate(strings.Repeat(" ", 6+4*item.depth)+item.note, model.listWidth()-1, "…")
		return paint(dimStyle, note+strings.Repeat(" ", max(model.listWidth()-lipgloss.Width(note), 0)))
	}
	todo := item.task
	marker := " "
	switch {
	case todo.CalendarPath == store.LocalCalendarPath:
	case todo.Conflict:
		marker = "!"
	case todo.ETag == "":
		marker = "+"
	case todo.Dirty:
		marker = "*"
	}
	summary := paint(base, todo.Summary)
	if todo.Done() || todo.Deleted {
		summary = paint(doneStyle, todo.Summary)
	}
	if todo.Deleted {
		summary += paint(dimStyle, " (deleted here)")
	}
	// an open task shows its date kind inside the checkbox, so on wide screens it is clear which task a date on the right belongs to
	now := time.Now()
	priority := priorityStyle[task.PriorityLabel(todo.Priority)]
	icon, iconStyle, date := " ", base, ""
	if todo.Due != nil {
		icon, iconStyle, date = "◷", dueStyle(todo, now), dateLabel(todo.Due, now)
	} else if todo.Start != nil {
		icon, iconStyle, date = "▷", dimStyle, dateLabel(todo.Start, now)
	}
	if todo.Done() {
		icon, iconStyle = "x", priority
	}
	left := paint(errorStyle, marker) + paint(base, " "+strings.Repeat("    ", item.depth)) +
		paint(priority, "[") + paint(iconStyle, icon) + paint(priority, "]") + paint(base, " ") + summary
	if item.hidden > 0 {
		left += paint(dimStyle, fmt.Sprintf(" ▸ %d", item.hidden))
	}
	if len(todo.Categories) > 0 {
		left += paint(base, " ")
	}
	for _, category := range todo.Categories {
		left += paint(base, " ") + paint(model.tagStyle(category), "#"+category)
	}

	right := ""
	if todo.Recurrence != "" {
		right += paint(dimStyle, "↻ ")
	}
	right += paint(iconStyle, date) + paint(base, " ")
	left = ansi.Truncate(left, max(model.listWidth()-lipgloss.Width(right)-1, 1), "…")
	return left + paint(base, strings.Repeat(" ", max(model.listWidth()-lipgloss.Width(left)-lipgloss.Width(right), 1))) + right
}
