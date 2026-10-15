package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/syncer"
	"github.com/puhi8/tasks/internal/task"
)

// calendarTasks returns the tasks of one list, or of all lists for an empty path.
func (model *Model) calendarTasks(calendarPath string) []task.Task {
	var tasks []task.Task
	for _, todo := range model.tasks {
		if calendarPath == "" || todo.CalendarPath == calendarPath {
			tasks = append(tasks, todo)
		}
	}
	return tasks
}

func (model *Model) addTask(parent *task.Task) {
	calendarPath := model.calendarPath
	if selected, ok := model.selected(); ok && calendarPath == "" {
		calendarPath = selected.CalendarPath
	}
	if calendarPath == "" && len(model.calendars) > 0 {
		calendarPath = model.calendars[0].Path
	}
	if parent != nil {
		calendarPath = parent.CalendarPath
	}
	if calendarPath == "" {
		model.message = "no list yet, press s to sync first"
		return
	}
	created := task.New(calendarPath, time.Now())
	if parent != nil {
		created.ParentUID = parent.UID
	}
	model.openEditor(created, true)
}

func (model *Model) toggleCompleted(selected task.Task) {
	edited := selected
	if selected.Done() {
		edited.Status = task.NeedsAction
	} else {
		var err error
		if edited, err = task.Complete(selected); err != nil {
			model.message = err.Error()
			return
		}
	}
	if model.saveTask(edited) && !edited.Done() && !selected.Done() {
		model.message = "repeating task moved to " + dateLabel(edited.Due, time.Now())
	}
}

// deleted rows stay until the deletion is synced, so u can restore them
func (model *Model) deleteTask(selected task.Task) {
	doomed := task.Subtree(model.tasks, selected)
	if model.hasConflict(doomed) {
		return
	}
	question := fmt.Sprintf("Delete “%s”?", selected.Summary)
	if len(doomed) > 1 {
		question = fmt.Sprintf("Delete “%s” and its %d subtask(s)?", selected.Summary, len(doomed)-1)
	}
	model.askConfirm(question, func() {
		model.lastDeleted = nil
		for _, current := range doomed {
			current.Deleted = true
			if err := model.database.SaveLocal(current); err != nil {
				model.message = "delete failed: " + err.Error()
				break
			}
			model.lastDeleted = append(model.lastDeleted, current.Path)
		}
		model.reload()
		model.message = fmt.Sprintf("deleted %d task(s), u to undo", len(model.lastDeleted))
	})
}

func (model *Model) deleteTag(tag string) {
	var tagged []task.Task
	for _, todo := range model.tasks {
		if containsFold(todo.Categories, tag) {
			tagged = append(tagged, todo)
		}
	}
	model.askConfirm(fmt.Sprintf("Remove tag #%s from %d task(s) in all lists?", tag, len(tagged)), func() {
		for _, todo := range tagged {
			edited := todo
			edited.Categories = slices.DeleteFunc(slices.Clone(todo.Categories), func(category string) bool { return strings.EqualFold(category, tag) })
			if _, err := model.applyAndSave(edited); err != nil {
				model.message = "removing tag failed: " + err.Error()
				model.reload()
				return
			}
		}
		if _, colored := model.tagColors[strings.ToLower(tag)]; colored {
			model.database.SetTagColor(tag, "")
		}
		model.reload()
		model.message = fmt.Sprintf("removed #%s from %d task(s)", tag, len(tagged))
	})
}

// on the server the list is deleted on the next sync
func (model *Model) deleteList(path string) {
	count := len(model.calendarTasks(path))
	name := model.calendarName(path)
	model.askConfirm(fmt.Sprintf("Delete list “%s” and its %d task(s)? It is deleted on the server too.", name, count), func() {
		if err := syncer.DeleteList(model.database, path); err != nil {
			model.message = "deleting list failed: " + err.Error()
			return
		}
		model.reload()
		model.message = "deleted list " + name
		if path != store.LocalCalendarPath && model.client != nil {
			model.message += ", removed from the server on the next sync"
		}
	})
}

func (model *Model) undoDelete() {
	restored := 0
	for _, path := range model.lastDeleted {
		deleted, err := model.database.Task(path)
		if err != nil || !deleted.Deleted {
			continue
		}
		deleted.Deleted = false
		if err := model.database.SaveLocal(deleted); err == nil {
			restored++
		}
	}
	if len(model.lastDeleted) > 0 && restored == 0 {
		model.message = "nothing to restore, the deletion was already synced"
	} else if restored > 0 {
		model.message = fmt.Sprintf("restored %d task(s)", restored)
	}
	model.lastDeleted = nil
	model.reload()
}

func (model *Model) indent(selected task.Task) {
	depth := model.rows[model.cursor].depth
	for index := model.cursor - 1; index >= 0; index-- {
		above := model.rows[index]
		if above.note != "" {
			continue
		}
		if above.header != "" || above.depth < depth {
			break
		}
		if above.depth == depth {
			model.setParent(selected, above.task.UID)
			return
		}
	}
	model.message = "no task above to indent under"
}

func (model *Model) outdent(selected task.Task) {
	if selected.ParentUID == "" {
		return
	}
	parent, _ := model.parentOf(selected)
	model.setParent(selected, parent.ParentUID)
}

// hasConflict refuses to delete or move tasks in conflict: the pending remote version would be left behind.
func (model *Model) hasConflict(tasks []task.Task) bool {
	if slices.ContainsFunc(tasks, func(todo task.Task) bool { return todo.Conflict }) {
		model.message = "resolve the conflict first: open the task marked ! with enter"
		return true
	}
	return false
}

func (model *Model) parentOf(child task.Task) (task.Task, bool) {
	for _, todo := range model.calendarTasks(child.CalendarPath) {
		if child.ParentUID != "" && todo.UID == child.ParentUID {
			return todo, true
		}
	}
	return task.Task{}, false
}

// toggleCollapsed on a subtask folds its parent instead, so tab works from anywhere inside a group.
func (model *Model) toggleCollapsed(selected task.Task) {
	target := selected
	hasChildren := slices.ContainsFunc(model.calendarTasks(selected.CalendarPath), func(todo task.Task) bool { return todo.ParentUID == selected.UID })
	if !hasChildren && !model.collapsed[selected.UID] {
		var ok bool
		if target, ok = model.parentOf(selected); !ok {
			return
		}
	}
	if err := model.database.SetCollapsed(target.UID, !model.collapsed[target.UID]); err != nil {
		model.message = err.Error()
		return
	}
	model.reload()
	model.selectPath(target.Path)
}

func (model *Model) setParent(selected task.Task, parentUID string) {
	if parentUID != "" && task.WouldCycle(model.calendarTasks(selected.CalendarPath), selected.UID, parentUID) {
		model.message = "a task cannot be moved under its own subtask"
		return
	}
	edited := selected
	edited.ParentUID = parentUID
	model.saveTask(edited)
}
