package tui

import (
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/puhi8/tasks/internal/task"
)

type row struct {
	header string // list name shown above its tasks in the all-lists view
	task   task.Task
	depth  int
	hidden int    // subtasks folded away under this task
	note   string // description line shown under its task
}

func (item row) isTask() bool { return item.header == "" && item.note == "" }

func (model *Model) updateList(key tea.KeyMsg) tea.Cmd {
	model.message = ""
	if action, found := model.listKeys[key.String()]; found {
		return action()
	}
	return nil
}

// listKeyTable maps every key of the task list to its action; "|" separates keys that share one.
func (model *Model) listKeyTable() map[string]func() tea.Cmd {
	run := func(action func()) func() tea.Cmd { return func() tea.Cmd { action(); return nil } }
	toggle := func(setting *bool) func() tea.Cmd { return run(func() { *setting = !*setting; model.refresh() }) }
	onSelected := func(action func(task.Task)) func() tea.Cmd {
		return run(func() {
			if selected, ok := model.selected(); ok {
				action(selected)
			}
		})
	}
	bindings := map[string]func() tea.Cmd{
		"q":             func() tea.Cmd { return tea.Quit },
		"j|down":        run(func() { model.move(1) }),
		"k|up":          run(func() { model.move(-1) }),
		"ctrl+d|pgdown": run(func() { model.move(model.bodyHeight() / 2) }),
		"ctrl+u|pgup":   run(func() { model.move(-model.bodyHeight() / 2) }),
		"g|home":        run(func() { model.move(-len(model.rows)) }),
		"G|end":         run(func() { model.move(len(model.rows)) }),
		"a":             run(func() { model.addTask(nil) }),
		"A":             onSelected(func(selected task.Task) { model.addTask(&selected) }),
		"enter":         onSelected(model.open),
		"tab":           onSelected(model.toggleCollapsed),
		"x| ":           onSelected(model.toggleCompleted),
		"d":             onSelected(model.deleteTask),
		"u":             run(model.undoDelete),
		"m":             onSelected(model.openMovePicker),
		"p":             onSelected(model.openParentPicker),
		">":             onSelected(model.indent),
		"<":             onSelected(model.outdent),
		"/":             model.openSearch,
		"esc":           run(func() { model.query = ""; model.refresh() }),
		"f":             run(model.openFilterPicker),
		"t":             run(model.openTagPicker),
		"l":             run(model.openListPicker),
		"n":             toggle(&model.showDescriptions),
		"h":             toggle(&model.showCompleted),
		"o":             toggle(&model.manualSort),
		"s":             model.startSync,
		"?":             run(func() { model.mode, model.helpOffset = helpMode, 0 }),
	}
	keys := map[string]func() tea.Cmd{}
	for names, action := range bindings {
		for _, name := range strings.Split(names, "|") {
			keys[name] = action
		}
	}
	return keys
}

func (model *Model) openSearch() tea.Cmd {
	model.mode = searchMode
	model.search.SetValue(model.query)
	model.search.CursorEnd()
	model.search.SetSuggestions(model.tagSuggestions(model.query))
	return model.search.Focus()
}

func (model *Model) open(selected task.Task) {
	if selected.Conflict {
		model.openConflict(selected)
	} else {
		model.openEditor(selected, false)
	}
}

func (model *Model) refresh() {
	selectedPath := ""
	if selected, ok := model.selected(); ok {
		selectedPath = selected.Path
	}
	now := time.Now()
	filter := parseQuery(model.query)
	byCalendar := map[string][]task.Task{}
	for _, todo := range model.calendarTasks(model.calendarPath) {
		if !model.showCompleted && filter.status == "" && todo.Done() {
			continue
		}
		if filter.match(todo, model.calendarName(todo.CalendarPath), now) {
			byCalendar[todo.CalendarPath] = append(byCalendar[todo.CalendarPath], todo)
		}
	}
	model.rows = nil
	var due [][]row
	for _, calendar := range model.calendars {
		tasks := byCalendar[calendar.Path]
		if len(tasks) == 0 {
			continue
		}
		dated, rest := model.splitDue(model.tree(tasks))
		due = append(due, dated...)
		// a single list only needs its name once the due area sits above it
		if len(rest) > 0 && (model.calendarPath == "" || len(dated) > 0) {
			model.rows = append(model.rows, row{header: calendar.Name})
		}
		model.rows = append(model.rows, rest...)
	}
	if len(due) > 0 {
		sort.SliceStable(due, func(i, j int) bool { return model.less(due[i][0].task, due[j][0].task) })
		model.rows = slices.Concat([]row{{header: "Due"}}, slices.Concat(due...), model.rows)
	}
	// when the selected task disappeared (completed, filtered out) keep the cursor where it was
	previous := model.cursor
	if !model.selectPath(selectedPath) {
		model.cursor = max(min(previous, len(model.rows)-1), 0)
		model.move(0)
	}
}

// tree orders tasks with subtasks under their parent; tasks whose parent is hidden, missing or in a loop show at the top level.
func (model *Model) tree(tasks []task.Task) []row {
	sort.SliceStable(tasks, func(i, j int) bool { return model.less(tasks[i], tasks[j]) })
	present := map[string]bool{}
	for _, todo := range tasks {
		present[todo.UID] = true
	}
	children := map[string][]task.Task{}
	for _, todo := range tasks {
		parent := todo.ParentUID
		if !present[parent] || parent == todo.UID {
			parent = ""
		}
		children[parent] = append(children[parent], todo)
	}
	var rows []row
	visited := map[string]bool{}
	// folded subtasks are marked visited too, so the loop fallback below does not show them at the top level
	var hide func(todo task.Task) int
	hide = func(todo task.Task) int {
		hidden := 0
		for _, child := range children[todo.UID] {
			if !visited[child.Path] {
				visited[child.Path] = true
				hidden += 1 + hide(child)
			}
		}
		return hidden
	}
	var walk func(todo task.Task, depth int)
	walk = func(todo task.Task, depth int) {
		if visited[todo.Path] {
			return
		}
		visited[todo.Path] = true
		item := row{task: todo, depth: depth}
		// a search shows everything, so matching subtasks are never folded away
		folded := model.collapsed[todo.UID] && model.query == ""
		if folded {
			item.hidden = hide(todo)
		}
		rows = append(rows, item)
		if note := strings.Join(strings.Fields(todo.Description), " "); model.showDescriptions && note != "" {
			rows = append(rows, row{note: note, depth: depth})
		}
		if folded {
			return
		}
		for _, child := range children[todo.UID] {
			walk(child, depth+1)
		}
	}
	for _, todo := range children[""] {
		walk(todo, 0)
	}
	for _, todo := range tasks {
		walk(todo, 0)
	}
	return rows
}

// splitDue takes the open top level tasks with a due date out of rows, each together with its subtasks and notes.
func (model *Model) splitDue(rows []row) (due [][]row, rest []row) {
	inDue := false
	for _, item := range rows {
		if item.isTask() && item.depth == 0 {
			inDue = !model.manualSort && item.task.Due != nil && !item.task.Done()
			if inDue {
				due = append(due, nil)
			}
		}
		if inDue {
			due[len(due)-1] = append(due[len(due)-1], item)
		} else {
			rest = append(rest, item)
		}
	}
	return due, rest
}

func (model *Model) less(first, second task.Task) bool {
	if first.Done() != second.Done() {
		return !first.Done()
	}
	if model.manualSort {
		if (first.SortOrder == nil) != (second.SortOrder == nil) {
			return first.SortOrder != nil
		}
		if first.SortOrder != nil && *first.SortOrder != *second.SortOrder {
			return *first.SortOrder < *second.SortOrder
		}
	} else {
		// a due date outranks any priority
		if (first.Due == nil) != (second.Due == nil) {
			return first.Due != nil
		}
		if first.Due != nil && !first.Due.Time.Equal(second.Due.Time) {
			return first.Due.Time.Before(second.Due.Time)
		}
		if priorityRank(first.Priority) != priorityRank(second.Priority) {
			return priorityRank(first.Priority) < priorityRank(second.Priority)
		}
	}
	return strings.ToLower(first.Summary) < strings.ToLower(second.Summary)
}

func priorityRank(priority int) int {
	if priority == 0 {
		return 10
	}
	return priority
}

func (model *Model) selected() (task.Task, bool) {
	if model.cursor < 0 || model.cursor >= len(model.rows) || !model.rows[model.cursor].isTask() {
		return task.Task{}, false
	}
	return model.rows[model.cursor].task, true
}

func (model *Model) selectPath(path string) bool {
	for index, row := range model.rows {
		if row.isTask() && row.task.Path == path {
			model.cursor = index
			model.scroll()
			return true
		}
	}
	return false
}

func (model *Model) move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	for moved := 0; moved != delta; moved += step {
		next := model.cursor + step
		for next >= 0 && next < len(model.rows) && !model.rows[next].isTask() {
			next += step
		}
		if next < 0 || next >= len(model.rows) {
			break
		}
		model.cursor = next
	}
	if model.cursor < len(model.rows) && !model.rows[model.cursor].isTask() && delta == 0 {
		model.move(1)
	}
	model.scroll()
}

func (model *Model) bodyHeight() int { return max(model.height-3, 1) }

func (model *Model) scroll() {
	height := model.bodyHeight()
	if model.cursor < model.offset {
		model.offset = model.cursor
	}
	if model.offset == model.cursor && model.cursor > 0 && model.rows[model.cursor-1].header != "" {
		model.offset--
	}
	// keep the selected task's description line on screen too
	last := model.cursor
	if last+1 < len(model.rows) && model.rows[last+1].note != "" {
		last++
	}
	if last >= model.offset+height {
		model.offset = last - height + 1
	}
	model.offset = max(min(model.offset, len(model.rows)-height), 0)
}
