package task

import (
	"errors"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	NeedsAction Status = "NEEDS-ACTION"
	Completed   Status = "COMPLETED"
	Cancelled   Status = "CANCELLED"
)

// List is a task list: a CalDAV collection on the server and a row in the local database.
type List struct {
	Path string
	Name string
	CTag string // changes whenever anything in the list changes on the server
}

type Date struct {
	Time   time.Time
	AllDay bool
}

// Sync is the local bookkeeping that never goes into the iCalendar data.
type Sync struct {
	Path         string // remote href, also the local key
	CalendarPath string
	ETag         string // empty until the task exists on the server

	Dirty           bool
	Deleted         bool
	Revision        int64 // bumped on every local save
	Conflict        bool
	RemoteICalendar string // remote version during a conflict, empty when it was deleted remotely
	RemoteETag      string
}

type Task struct {
	Sync

	UID         string
	Summary     string
	Description string
	Status      Status
	Priority    int // 0 undefined, 1 highest, 9 lowest

	Start    *Date
	Due      *Date
	Modified *Date

	Location   string
	Categories []string
	ParentUID  string
	SortOrder  *int64
	Recurrence string // raw RRULE value

	// RawICalendar is the source of truth; every field above is derived from it.
	RawICalendar string
}

func (task Task) Done() bool { return task.Status == Completed || task.Status == Cancelled }

func PriorityLabel(priority int) string {
	switch {
	case priority == 0:
		return "none"
	case priority < 5:
		return "high"
	case priority == 5:
		return "medium"
	}
	return "low"
}

func Parse(raw string) (Task, error) {
	_, todo, err := parseTodo(raw)
	if err != nil {
		return Task{}, err
	}
	parsed := read(todo)
	parsed.RawICalendar = raw
	return parsed, nil
}

// Restore rebuilds a stored or downloaded task; data no client can parse is still listed under its file name.
func Restore(sync Sync, raw string) (Task, error) {
	restored, err := Parse(raw)
	if err != nil {
		restored = Task{Summary: "(unreadable) " + path.Base(sync.Path), Status: NeedsAction, RawICalendar: raw}
	}
	if restored.UID == "" {
		restored.UID = path.Base(sync.Path)
	}
	restored.Sync = sync
	return restored, err
}

func New(calendarPath string, now time.Time) Task {
	uid := uuid.NewString()
	stamp := now.UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//tasks-tui//EN",
		"BEGIN:VTODO", "UID:" + uid, "DTSTAMP:" + stamp, "CREATED:" + stamp, "LAST-MODIFIED:" + stamp, "STATUS:NEEDS-ACTION",
		"END:VTODO", "END:VCALENDAR", ""}
	created, _ := Parse(strings.Join(lines, "\r\n"))
	created.CalendarPath = calendarPath
	created.Path = PathFor(calendarPath, uid)
	return created
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// PathFor names a task resource after its UID when that is URL-safe, so the server echoes the same href back.
func PathFor(calendarPath, uid string) string {
	if !safeName.MatchString(uid) {
		uid = uuid.NewString()
	}
	return strings.TrimSuffix(calendarPath, "/") + "/" + uid + ".ics"
}

// Apply patches only the changed fields into RawICalendar, so everything this app does not understand survives.
func Apply(edited Task, now time.Time) (Task, error) {
	calendar, todo, err := parseTodo(edited.RawICalendar)
	if err != nil {
		return edited, err
	}
	if !patch(todo, read(todo), edited, now) {
		return edited, nil
	}
	updated, err := Parse(calendar.String())
	if err != nil {
		return edited, err
	}
	updated.Sync = edited.Sync
	return updated, nil
}

func Validate(task Task) error {
	if strings.TrimSpace(task.Summary) == "" {
		return errors.New("title is required")
	}
	if task.Recurrence != "" && !strings.Contains(strings.ToUpper(task.Recurrence), "FREQ=") {
		return errors.New("repeat rule needs FREQ=, e.g. FREQ=WEEKLY")
	}
	return nil
}

// Subtree returns root followed by every subtask below it; the visited set keeps parent loops from other clients finite.
func Subtree(tasks []Task, root Task) []Task {
	children := map[string][]Task{}
	for _, task := range tasks {
		if task.CalendarPath == root.CalendarPath && !task.Deleted {
			children[task.ParentUID] = append(children[task.ParentUID], task)
		}
	}
	var subtree []Task
	queue := []Task{root}
	visited := map[string]bool{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if !visited[current.Path] {
			visited[current.Path] = true
			subtree = append(subtree, current)
			queue = append(queue, children[current.UID]...)
		}
	}
	return subtree
}

func WouldCycle(tasks []Task, uid, parentUID string) bool {
	parents := map[string]string{}
	for _, task := range tasks {
		parents[task.UID] = task.ParentUID
	}
	for current, steps := parentUID, 0; current != "" && steps <= len(tasks); current, steps = parents[current], steps+1 {
		if current == uid {
			return true
		}
	}
	return false
}
