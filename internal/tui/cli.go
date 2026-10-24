package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/puhi8/tasks/internal/config"
	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/task"
)

// idWidth is how much of a task's UID tasks list prints; done and delete accept any unique start of it.
const idWidth = 8

// Command runs one plain terminal command on the local copy; changed tells the caller there is something to upload.
func Command(database *store.Store, settings config.Config, listName, due string, words []string) (changed bool, err error) {
	model := New(database, nil, settings, listName, nil)
	if listName != "" && model.calendarPath == "" {
		return false, fmt.Errorf("no list named %q, see tasks lists", listName)
	}
	if model.message != "" {
		return false, errors.New(model.message)
	}
	arguments := words[1:]
	switch words[0] {
	case "list":
		model.printList(strings.Join(arguments, " "))
		return false, nil
	case "lists":
		for _, calendar := range model.calendars {
			fmt.Println(calendar.Name)
		}
		return false, nil
	case "add":
		return true, model.addFromCommand(strings.Join(arguments, " "), due)
	case "show":
		if len(arguments) == 0 {
			return false, errNoTask
		}
		for _, word := range arguments {
			selected, err := model.find(word)
			if err != nil {
				return false, err
			}
			model.printTree(selected)
		}
		return false, nil
	case "done":
		return model.changeEach(arguments, model.completeFromCommand)
	case "delete":
		return model.changeEach(arguments, model.deleteFromCommand)
	case "__ids":
		// read by the shell completion, so done and delete can complete ids with the titles shown next to them
		for _, todo := range model.calendarTasks(model.calendarPath) {
			if !todo.Done() {
				fmt.Printf("%s\t%s\n", shortID(todo.UID), todo.Summary)
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("unknown command %q, see tasks --help", words[0])
}

// printList prints the tasks in the app's order, with nothing folded away.
func (model *Model) printList(query string) {
	if width, _, err := term.GetSize(os.Stdout.Fd()); err == nil {
		model.width = width
	}
	// staying below previewMinWidth keeps the rows from leaving room for the preview pane
	model.width = min(model.width, previewMinWidth-1) - idWidth - 1
	model.query, model.collapsed = query, map[string]bool{}
	model.refresh()
	for _, item := range model.rows {
		prefix := ""
		if item.isTask() {
			prefix = dimStyle.Render(fmt.Sprintf("%-*s", idWidth, shortID(item.task.UID))) + " "
		} else if item.note != "" {
			prefix = strings.Repeat(" ", idWidth+1)
		}
		fmt.Println(prefix + model.renderRow(item, false))
	}
}

func shortID(uid string) string { return uid[:min(len(uid), idWidth)] }

// printTree prints a task with all its subtasks as a Markdown checklist, descriptions in full, so it can be read as a whole.
func (model *Model) printTree(root task.Task) {
	model.collapsed = map[string]bool{}
	for _, item := range model.tree(task.Subtree(model.tasks, root)) {
		if !item.isTask() {
			continue
		}
		indent := strings.Repeat("  ", item.depth)
		fmt.Println(indent + "- " + checklistLine(item.task))
		if description := strings.TrimSpace(item.task.Description); description != "" {
			for _, line := range strings.Split(description, "\n") {
				fmt.Println(strings.TrimRight(indent+"  "+line, " \r"))
			}
		}
	}
}

func checklistLine(todo task.Task) string {
	box := "[ ]"
	if todo.Done() {
		box = "[x]"
	}
	details := []string{"id " + shortID(todo.UID)}
	if todo.Due != nil {
		details = append(details, "due "+formatDateInput(todo.Due))
	}
	if todo.Priority != 0 {
		details = append(details, task.PriorityLabel(todo.Priority)+" priority")
	}
	for _, category := range todo.Categories {
		details = append(details, "#"+category)
	}
	return box + " " + todo.Summary + " (" + strings.Join(details, ", ") + ")"
}

func (model *Model) addFromCommand(title, due string) error {
	calendarPath := model.calendarPath
	if calendarPath == "" && len(model.calendars) == 1 {
		calendarPath = model.calendars[0].Path
	}
	switch {
	case len(model.calendars) == 0:
		return errors.New("no list yet, run tasks sync first")
	case calendarPath == "":
		return errors.New("there are several lists, choose one with --list NAME (see tasks lists)")
	}
	created := task.New(calendarPath, time.Now())
	created.Summary = strings.TrimSpace(title)
	var err error
	if created.Due, err = parseDateInput(due, nil); err != nil {
		return fmt.Errorf("--due: %w", err)
	}
	if err := task.Validate(created); err != nil {
		return err
	}
	saved, err := model.applyAndSave(created)
	if err == nil {
		fmt.Println("added", shortID(saved.UID), saved.Summary)
	}
	return err
}

var errNoTask = errors.New("name the task by its id from tasks list or by its title")

func (model *Model) changeEach(words []string, change func(task.Task) (string, error)) (changed bool, err error) {
	if len(words) == 0 {
		return false, errNoTask
	}
	for _, word := range words {
		selected, err := model.find(word)
		if err != nil {
			return changed, err
		}
		report, err := change(selected)
		if err != nil {
			return changed, err
		}
		fmt.Println(report)
		changed = true
		model.reload()
	}
	return changed, nil
}

// find takes the start of an id as tasks list prints it, or a whole title.
func (model *Model) find(word string) (task.Task, error) {
	var found []task.Task
	for _, todo := range model.calendarTasks(model.calendarPath) {
		if strings.HasPrefix(todo.UID, word) || strings.EqualFold(todo.Summary, word) {
			found = append(found, todo)
		}
	}
	switch {
	case word == "" || len(found) > 1:
		return task.Task{}, fmt.Errorf("%q matches %d tasks, use the id from tasks list", word, len(found))
	case len(found) == 0:
		return task.Task{}, fmt.Errorf("no task %q", word)
	}
	return found[0], nil
}

func (model *Model) completeFromCommand(selected task.Task) (string, error) {
	edited, err := task.Complete(selected)
	if err == nil {
		_, err = model.applyAndSave(edited)
	}
	if edited.Done() {
		return "completed " + selected.Summary, err
	}
	return selected.Summary + " repeats, moved to " + dateLabel(edited.Due, time.Now()), err
}

func (model *Model) deleteFromCommand(selected task.Task) (string, error) {
	doomed := task.Subtree(model.tasks, selected)
	if model.hasConflict(doomed) {
		return "", errors.New(model.message)
	}
	report := "deleted " + selected.Summary
	if len(doomed) > 1 {
		report += fmt.Sprintf(" and its %d subtask(s)", len(doomed)-1)
	}
	return report, model.markDeleted(doomed)
}
