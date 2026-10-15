package store

import (
	"database/sql"
	"strings"

	"github.com/puhi8/tasks/internal/task"
)

// the stored iCalendar is the only copy of the task fields; the rest is sync bookkeeping
var taskColumns = []string{"path", "calendar_path", "etag", "raw_ical", "dirty", "deleted", "revision", "conflict", "remote_ical", "remote_etag"}

var (
	selectTasks = "SELECT " + strings.Join(taskColumns, ", ") + " FROM tasks"
	saveQuery   = upsertQuery("excluded.revision", "")
	// saveLocalQuery records a user edit: the revision bump lets sync notice edits made while it was working.
	saveLocalQuery = upsertQuery("tasks.revision + 1", "")
	// saveRemoteQuery never overwrites a task with unsynced local changes.
	saveRemoteQuery = upsertQuery("tasks.revision", " WHERE tasks.dirty = 0 AND tasks.conflict = 0")
)

func upsertQuery(revision, condition string) string {
	var assignments []string
	for _, column := range taskColumns[1:] {
		value := "excluded." + column
		if column == "revision" {
			value = revision
		}
		assignments = append(assignments, column+" = "+value)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(taskColumns)), ", ")
	return "INSERT INTO tasks (" + strings.Join(taskColumns, ", ") + ") VALUES (" + placeholders +
		") ON CONFLICT(path) DO UPDATE SET " + strings.Join(assignments, ", ") + condition
}

// Tasks returns every task, including deleted ones still waiting to be synced.
func (store *Store) Tasks() ([]task.Task, error) { return store.queryTasks("") }

func (store *Store) CalendarTasks(calendarPath string) ([]task.Task, error) {
	return store.queryTasks(" WHERE calendar_path = ?", calendarPath)
}

func (store *Store) Task(path string) (task.Task, error) {
	tasks, err := store.queryTasks(" WHERE path = ?", path)
	if err != nil {
		return task.Task{}, err
	}
	if len(tasks) == 0 {
		return task.Task{}, sql.ErrNoRows
	}
	return tasks[0], nil
}

func (store *Store) HasDirty() (bool, error) {
	var count int
	err := store.database.QueryRow("SELECT COUNT(*) FROM tasks WHERE dirty = 1 AND calendar_path != ?", LocalCalendarPath).Scan(&count)
	return count > 0, err
}

func (store *Store) Save(record task.Task) error { return store.exec(saveQuery, values(record)...) }

func (store *Store) SaveLocal(record task.Task) error {
	record.Dirty = true
	return store.exec(saveLocalQuery, values(record)...)
}

func (store *Store) SaveRemote(record task.Task) error {
	record.Dirty, record.Deleted, record.Conflict = false, false, false
	return store.exec(saveRemoteQuery, values(record)...)
}

func (store *Store) SetConflict(path, remoteICalendar, remoteETag string) error {
	return store.exec("UPDATE tasks SET conflict = 1, remote_ical = ?, remote_etag = ? WHERE path = ?", remoteICalendar, remoteETag, path)
}

// MarkPushed stores the new ETag and clears dirty unless the task was edited again during the upload.
func (store *Store) MarkPushed(path string, revision int64, etag string) error {
	return store.exec("UPDATE tasks SET etag = ?, dirty = (revision != ?) WHERE path = ?", etag, revision, path)
}

// Remove deletes the row unless it was edited locally after revision was read.
func (store *Store) Remove(path string, revision int64) error {
	return store.exec("DELETE FROM tasks WHERE path = ? AND revision = ?", path, revision)
}

func (store *Store) queryTasks(where string, arguments ...any) ([]task.Task, error) {
	return collect(store, func(rows *sql.Rows) (task.Task, error) {
		var sync task.Sync
		var raw string
		err := rows.Scan(&sync.Path, &sync.CalendarPath, &sync.ETag, &raw, &sync.Dirty, &sync.Deleted, &sync.Revision, &sync.Conflict, &sync.RemoteICalendar, &sync.RemoteETag)
		restored, _ := task.Restore(sync, raw)
		return restored, err
	}, selectTasks+where, arguments...)
}

func values(record task.Task) []any {
	return []any{record.Path, record.CalendarPath, record.ETag, record.RawICalendar, record.Dirty, record.Deleted, record.Revision, record.Conflict, record.RemoteICalendar, record.RemoteETag}
}
