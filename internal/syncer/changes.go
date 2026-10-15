package syncer

import (
	"time"

	"github.com/puhi8/tasks/internal/caldav"
	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/task"
)

// DeleteList removes a list with its tasks; lists on the server are deleted there on the next sync.
func DeleteList(database *store.Store, path string) error {
	if path == store.LocalCalendarPath {
		return database.DeleteCalendar(path)
	}
	return database.MarkCalendarDeleted(path)
}

// KeepLocal resolves a conflict in favour of the local version; the next sync overwrites the remote copy.
func KeepLocal(database *store.Store, conflicted task.Task) error {
	conflicted.ETag = conflicted.RemoteETag
	conflicted.Conflict, conflicted.RemoteICalendar, conflicted.RemoteETag = false, "", ""
	return database.SaveLocal(conflicted)
}

func KeepRemote(database *store.Store, conflicted task.Task) error {
	if conflicted.RemoteICalendar == "" {
		return database.Remove(conflicted.Path, conflicted.Revision)
	}
	remote := fromRemote(caldav.RemoteTask{Path: conflicted.Path, ETag: conflicted.RemoteETag, Data: conflicted.RemoteICalendar}, conflicted.CalendarPath, &Result{})
	remote.Revision = conflicted.Revision
	return database.Save(remote)
}

// Move re-creates a task (and its subtasks) in another list and deletes the original, like Tasks.org does.
func Move(database *store.Store, tasks []task.Task, moved task.Task, calendarPath string) error {
	for _, current := range task.Subtree(tasks, moved) {
		copied := current
		copied.Sync = task.Sync{Path: task.PathFor(calendarPath, current.UID), CalendarPath: calendarPath}
		// moving back before a sync lands on the pending tombstone: reuse its ETag so the upload is a plain update
		if existing, err := database.Task(copied.Path); err == nil {
			copied.ETag, copied.Revision = existing.ETag, existing.Revision
		}
		if current.Path == moved.Path && current.ParentUID != "" {
			copied.ParentUID = ""
			updated, err := task.Apply(copied, time.Now())
			if err != nil {
				return err
			}
			copied = updated
		}
		if err := database.SaveLocal(copied); err != nil {
			return err
		}
		current.Deleted = true
		if err := database.SaveLocal(current); err != nil {
			return err
		}
	}
	return nil
}
