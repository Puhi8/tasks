package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/puhi8/tasks/internal/caldav"
	"github.com/puhi8/tasks/internal/config"
	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/task"
)

type mode int

const (
	listMode mode = iota
	searchMode
	editMode
	pickMode
	conflictMode
	helpMode
	confirmMode
)

type confirmation struct {
	question  string
	action    func()
	askedFrom mode
}

var (
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#a78bfa"})
	titleStyle    = accentStyle.Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"})
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	doneStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true)
	priorityStyle = map[string]lipgloss.Style{
		"high":   errorStyle,
		"medium": warningStyle,
		"low":    lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		"none":   lipgloss.NewStyle().Foreground(lipgloss.Color("8"))}
)

const doubleEscape = 500 * time.Millisecond

type Model struct {
	database *store.Store
	client   *caldav.Client

	calendars []task.List
	tasks     []task.Task // everything not deleted
	tagColors map[string]string
	collapsed map[string]bool
	pending   int
	rows      []row
	cursor    int
	offset    int
	width     int
	height    int

	calendarPath     string // empty shows all lists
	query            string
	showCompleted    bool
	showDescriptions bool
	manualSort       bool

	mode       mode
	search     textinput.Model
	editor     editor
	preview    editor
	previewKey string
	picker     picker
	conflict   conflictView
	confirm    confirmation

	message       string
	syncing       bool
	syncStatus    string
	syncOff       string
	syncOnStart   bool
	syncInterval  time.Duration
	pushScheduled bool
	syncFailed    bool
	stuckPending  int
	lastDeleted   []string
	lastEscape    time.Time
	listKeys      map[string]func() tea.Cmd
	helpOffset    int
}

// New takes syncOff when a server is configured but cannot be used; the app then works offline and says why.
func New(database *store.Store, client *caldav.Client, settings config.Config, listName string, syncOff error) *Model {
	search := textinput.New()
	search.Prompt = "/"
	search.ShowSuggestions = true
	model := &Model{database: database, client: client, search: search, width: 80, height: 24,
		syncOnStart: settings.Sync.SyncOnStart, syncInterval: time.Duration(settings.Sync.IntervalMinutes) * time.Minute,
		showDescriptions: settings.UI.ShowDescriptions}
	model.listKeys = model.listKeyTable()
	if client == nil {
		model.syncStatus = "local only"
	}
	if syncOff != nil {
		model.syncStatus, model.syncOff = "sync off", "sync off: "+syncOff.Error()
	}
	model.reload()
	for _, calendar := range model.calendars {
		if listName != "" && strings.EqualFold(calendar.Name, listName) {
			model.calendarPath = calendar.Path
		}
	}
	if listName != "" && model.calendarPath == "" {
		model.message = fmt.Sprintf("no list named %q yet, showing all", listName)
	}
	model.refresh()
	if model.syncOff != "" {
		model.message = model.syncOff
	}
	return model
}

func (model *Model) Init() tea.Cmd {
	if model.syncOnStart && model.client != nil {
		return tea.Batch(model.startSync(), model.scheduleAutoSync())
	}
	return model.scheduleAutoSync()
}

func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		if model.mode == editMode {
			model.editor.resize(model.editorWidth(), model.height)
		}
		model.scroll()
		return model, nil
	case tea.MouseMsg:
		return model, tea.Batch(model.updateMouse(message), model.schedulePush())
	case syncDone:
		model.finishSync(message)
		return model, model.schedulePush()
	case tea.KeyMsg:
		return model, tea.Batch(model.updateKey(message), model.schedulePush())
	case autoSyncDue:
		return model, tea.Batch(model.startSync(), model.scheduleAutoSync())
	case pushDue:
		model.pushScheduled = false
		return model, model.startSync()
	}
	// cursor blinking and other internal messages go to the focused input
	var command tea.Cmd
	if model.mode == searchMode {
		model.search, command = model.search.Update(message)
	} else if model.mode == editMode {
		command = model.editor.update(message)
	}
	return model, command
}

func (model *Model) updateKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "ctrl+c" {
		return tea.Quit
	}
	// esc twice quickly quits from anywhere; the first esc still does its job (an open editor is saved)
	if key.String() == "alt+esc" || (key.String() == "esc" && time.Since(model.lastEscape) < doubleEscape) {
		if model.mode == editMode {
			model.saveEditor()
		}
		return tea.Quit
	}
	if key.String() == "esc" {
		model.lastEscape = time.Now()
	}
	// Bubble Tea delivers fast typing (held keys) as one multi-rune message; outside text inputs each rune is a command
	if key.Type == tea.KeyRunes && len(key.Runes) > 1 && !key.Paste && model.mode != searchMode && model.mode != editMode {
		var commands []tea.Cmd
		for _, character := range key.Runes {
			command := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
			commands = append(commands, command)
		}
		return tea.Batch(commands...)
	}
	switch model.mode {
	case searchMode:
		return model.updateSearch(key)
	case editMode:
		return model.updateEditor(key)
	case pickMode:
		return model.updatePicker(key)
	case conflictMode:
		return model.updateConflict(key)
	case helpMode:
		switch key.String() {
		case "j", "down":
			model.helpOffset++
		case "k", "up":
			model.helpOffset = max(model.helpOffset-1, 0)
		default:
			model.mode = listMode
		}
		return nil
	case confirmMode:
		if key.String() == "y" {
			model.mode = listMode
			model.confirm.action()
		} else {
			model.mode = model.confirm.askedFrom
			model.message = "cancelled"
		}
		return nil
	}
	return model.updateList(key)
}

func (model *Model) askConfirm(question string, action func()) {
	model.confirm = confirmation{question: question, action: action, askedFrom: model.mode}
	model.mode = confirmMode
}

func (model *Model) View() string {
	if model.mode == confirmMode {
		model.mode = model.confirm.askedFrom
		lines := strings.Split(model.View(), "\n")
		model.mode = confirmMode
		lines[len(lines)-1] = errorStyle.Render(" " + model.confirm.question + " [y/N]")
		return strings.Join(lines, "\n")
	}
	wide := model.previewWidth() > 0
	switch {
	case model.mode == editMode && wide:
		return model.split(model.renderEditor(&model.editor, model.editorWidth()))
	case model.mode == editMode:
		return model.renderEditor(&model.editor, model.width)
	case model.mode == pickMode:
		return model.viewPicker()
	case model.mode == conflictMode:
		return model.viewConflict()
	case model.mode == helpMode:
		return model.viewHelp()
	case wide:
		return model.split(model.previewPane())
	}
	return model.viewList()
}

func (model *Model) reload() {
	tasks, err := model.load()
	if err != nil {
		model.message = err.Error()
		return
	}
	model.tasks, model.pending = nil, 0
	for _, todo := range tasks {
		if todo.Dirty && !todo.Conflict && todo.CalendarPath != store.LocalCalendarPath {
			model.pending++
		}
		// a deleted task stays visible while it has a conflict, otherwise it could never be resolved;
		// tasks of a list waiting to be deleted are hidden with it
		if (!todo.Deleted || todo.Conflict) && model.calendarName(todo.CalendarPath) != "" {
			model.tasks = append(model.tasks, todo)
		}
	}
	if model.calendarName(model.calendarPath) == "" {
		model.calendarPath = ""
	}
	model.refresh()
}

func (model *Model) load() (tasks []task.Task, err error) {
	if model.calendars, err = model.database.Calendars(); err != nil {
		return nil, err
	}
	if model.tagColors, err = model.database.TagColors(); err != nil {
		return nil, err
	}
	if model.collapsed, err = model.database.Collapsed(); err != nil {
		return nil, err
	}
	return model.database.Tasks()
}

func (model *Model) calendarName(path string) string {
	for _, calendar := range model.calendars {
		if calendar.Path == path {
			return calendar.Name
		}
	}
	return ""
}

// applyAndSave leaves an unchanged task alone, so it is not uploaded again for nothing.
func (model *Model) applyAndSave(edited task.Task) (task.Task, error) {
	updated, err := task.Apply(edited, time.Now())
	if err != nil || updated.RawICalendar == edited.RawICalendar {
		return updated, err
	}
	return updated, model.database.SaveLocal(updated)
}

func (model *Model) saveTask(edited task.Task) bool {
	updated, err := model.applyAndSave(edited)
	if err != nil {
		model.message = "could not save: " + err.Error()
		return false
	}
	model.reload()
	model.selectPath(updated.Path)
	return true
}
