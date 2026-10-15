package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/puhi8/tasks/internal/ical"
	"github.com/puhi8/tasks/internal/syncer"
	"github.com/puhi8/tasks/internal/task"
)

// conflictKeys is the last line of the conflict screen; the mouse handler finds the options in it.
const conflictKeys = " [l] keep local  [r] keep remote  [v] view differences  [c] cancel"

type conflictView struct {
	task     task.Task
	showDiff bool
}

func (model *Model) openConflict(todo task.Task) {
	model.conflict = conflictView{task: todo}
	model.mode = conflictMode
}

func (model *Model) updateConflict(key tea.KeyMsg) tea.Cmd {
	conflicted := model.conflict.task
	var err error
	switch key.String() {
	case "l":
		err = syncer.KeepLocal(model.database, conflicted)
		model.message = "kept local version, it is uploaded on the next sync"
	case "r":
		err = syncer.KeepRemote(model.database, conflicted)
		model.message = "kept remote version"
	case "v":
		model.conflict.showDiff = !model.conflict.showDiff
		return nil
	case "c", "esc", "q":
		model.mode = listMode
		return nil
	default:
		return nil
	}
	if err != nil {
		model.message = "could not resolve: " + err.Error()
	}
	model.mode = listMode
	model.reload()
	return nil
}

func (model *Model) viewConflict() string {
	local := model.conflict.task
	lines := []string{errorStyle.Render(" Conflict: ") + local.Summary, ""}
	remoteDeleted := local.RemoteICalendar == ""
	switch {
	case model.conflict.showDiff && !remoteDeleted:
		lines = append(lines, dimStyle.Render(" - only in local   + only on server"), "")
		lines = append(lines, diffLines(local.RawICalendar, local.RemoteICalendar)...)
	default:
		lines = append(lines, titleStyle.Render(" Local"))
		if local.Deleted {
			lines = append(lines, "   deleted here")
		} else {
			lines = append(lines, describe(local)...)
		}
		lines = append(lines, "", titleStyle.Render(" Server"))
		if remote, err := task.Parse(local.RemoteICalendar); remoteDeleted {
			lines = append(lines, "   deleted on the server")
		} else if err != nil {
			lines = append(lines, "   unreadable: "+err.Error())
		} else {
			lines = append(lines, describe(remote)...)
		}
	}
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], model.width-1, "…")
	}
	height := max(model.height-3, 1)
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n") + "\n\n" + dimStyle.Render(conflictKeys)
}

func describe(todo task.Task) []string {
	lines := []string{"   Title     " + todo.Summary, "   Done      " + map[bool]string{true: "yes", false: "no"}[todo.Done()]}
	if todo.Due != nil {
		lines = append(lines, "   Due       "+formatDateInput(todo.Due))
	}
	if todo.Priority != 0 {
		lines = append(lines, "   Priority  "+task.PriorityLabel(todo.Priority))
	}
	if len(todo.Categories) > 0 {
		lines = append(lines, "   Tags      "+strings.Join(todo.Categories, ", "))
	}
	if todo.Modified != nil {
		lines = append(lines, "   Modified  "+formatDateInput(todo.Modified))
	}
	return lines
}

// diffLines lists content lines present on only one side; enough to see what changed in a task.
func diffLines(local, remote string) []string {
	var result []string
	for _, side := range [][3]string{{errorStyle.Render(" - "), local, remote}, {titleStyle.Render(" + "), remote, local}} {
		others := map[string]int{}
		for _, line := range ical.Unfold(side[2]) {
			others[line]++
		}
		for _, line := range ical.Unfold(side[1]) {
			if others[line] > 0 {
				others[line]--
			} else if strings.TrimSpace(line) != "" {
				result = append(result, side[0]+line)
			}
		}
	}
	if len(result) == 0 {
		result = append(result, "   no differences in content")
	}
	return result
}
