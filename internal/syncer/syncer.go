package syncer

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/puhi8/tasks/internal/caldav"
	"github.com/puhi8/tasks/internal/ical"
	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/task"
)

type Result struct {
	Pulled    int
	Pushed    int
	Deleted   int
	Conflicts int
	Warnings  []string
}

// running keeps a background sync and the sync on exit from uploading the same task twice.
var running sync.Mutex

func Run(client *caldav.Client, database *store.Store) (Result, error) {
	running.Lock()
	defer running.Unlock()
	var result Result
	deleted, err := database.DeletedCalendars()
	if err != nil {
		return result, err
	}
	pending := map[string]bool{}
	for _, path := range deleted {
		if err := client.Delete(path, ""); err != nil {
			pending[path] = true
			result.Warnings = append(result.Warnings, "deleting list: "+err.Error())
		} else if err := database.DeleteCalendar(path); err != nil {
			return result, err
		}
	}
	collections, err := client.Discover()
	if err != nil {
		return result, err
	}
	storedCTags, err := updateCalendars(database, collections)
	if err != nil {
		return result, err
	}
	if err := syncTagColors(client, database); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	var failures []error
	for _, collection := range collections {
		if pending[collection.Path] {
			continue
		}
		if err := syncCollection(client, database, collection, storedCTags[collection.Path], &result); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", collection.Name, err))
		}
	}
	return result, errors.Join(failures...)
}

// updateCalendars returns the ctags stored before this sync; they only advance once a collection itself synced.
func updateCalendars(database *store.Store, collections []task.List) (map[string]string, error) {
	calendars, err := database.Calendars()
	if err != nil {
		return nil, err
	}
	storedCTags := map[string]string{}
	for _, calendar := range calendars {
		storedCTags[calendar.Path] = calendar.CTag
	}
	remote := map[string]bool{}
	for _, collection := range collections {
		remote[collection.Path] = true
		known := collection
		known.CTag = storedCTags[collection.Path]
		if err := database.SaveCalendar(known); err != nil {
			return nil, err
		}
	}
	for _, calendar := range calendars {
		if remote[calendar.Path] || calendar.Path == store.LocalCalendarPath {
			continue
		}
		tasks, err := database.CalendarTasks(calendar.Path)
		if err != nil {
			return nil, err
		}
		if !anyDirty(tasks) {
			if err := database.DeleteCalendar(calendar.Path); err != nil {
				return nil, err
			}
		}
	}
	return storedCTags, nil
}

// ponytail: last writer wins per tag (WebDAV properties have no ETag); fine for colors
func syncTagColors(client *caldav.Client, database *store.Store) error {
	remote, err := client.TagColors()
	if err != nil {
		return err
	}
	local, err := database.DirtyTagColors()
	if err != nil {
		return err
	}
	if len(local) > 0 {
		for tag, color := range local {
			if color == "" {
				delete(remote, tag)
			} else {
				remote[tag] = color
			}
		}
		if err := client.SetTagColors(remote); err != nil {
			return err
		}
	}
	return database.ReplaceTagColors(remote, local)
}

func syncCollection(client *caldav.Client, database *store.Store, collection task.List, storedCTag string, result *Result) error {
	local, err := database.CalendarTasks(collection.Path)
	if err != nil {
		return err
	}
	if collection.CTag != "" && storedCTag == collection.CTag && !anyDirty(local) {
		return nil
	}
	if err := pull(client, database, collection, local, result); err != nil {
		return err
	}
	written := result.Pushed + result.Deleted
	if err := push(client, database, collection, result); err != nil {
		return err
	}
	// our own writes changed the collection, so the ctag seen before them is stale (Radicale derives it from content and can repeat it)
	if result.Pushed+result.Deleted != written {
		collection.CTag = ""
	}
	return database.SaveCalendar(collection)
}

func pull(client *caldav.Client, database *store.Store, collection task.List, local []task.Task, result *Result) error {
	remote, err := client.ListTasks(collection.Path)
	if err != nil {
		return err
	}
	remoteETags := map[string]string{}
	for _, item := range remote {
		remoteETags[item.Path] = item.ETag
	}
	localTasks := map[string]task.Task{}
	var download []string
	for _, localTask := range local {
		localTasks[localTask.Path] = localTask
		etag, exists := remoteETags[localTask.Path]
		switch {
		case localTask.ETag == "":
			// never uploaded; push creates it
		case localTask.Conflict && etag == localTask.RemoteETag && (exists || localTask.RemoteICalendar == ""):
			// conflict already records this remote state
		case !exists && (localTask.Deleted || !localTask.Dirty):
			if err := database.Remove(localTask.Path, localTask.Revision); err != nil {
				return err
			}
			result.Deleted++
		case !exists:
			if err := conflict(database, localTask.Path, "", "", result); err != nil {
				return err
			}
		case etag != localTask.ETag:
			download = append(download, localTask.Path)
		}
	}
	for _, item := range remote {
		if _, known := localTasks[item.Path]; !known {
			download = append(download, item.Path)
		}
	}
	fetched, err := client.FetchTasks(collection.Path, download)
	if err != nil {
		return err
	}
	for _, item := range fetched {
		localTask, known := localTasks[item.Path]
		if known && localTask.Dirty && !localTask.Deleted && !localTask.Conflict && sameContent(localTask.RawICalendar, item.Data) {
			if err := database.MarkPushed(item.Path, localTask.Revision, item.ETag); err != nil {
				return err
			}
			continue
		}
		if known && (localTask.Dirty || localTask.Conflict) {
			if err := conflict(database, item.Path, item.Data, item.ETag, result); err != nil {
				return err
			}
			continue
		}
		if err := database.SaveRemote(fromRemote(item, collection.Path, result)); err != nil {
			return err
		}
		result.Pulled++
	}
	return nil
}

func push(client *caldav.Client, database *store.Store, collection task.List, result *Result) error {
	local, err := database.CalendarTasks(collection.Path)
	if err != nil {
		return err
	}
	for _, localTask := range local {
		if !localTask.Dirty || localTask.Conflict {
			continue
		}
		if err := pushTask(client, database, localTask, result); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%q: %v", localTask.Summary, err))
		}
	}
	return nil
}

func pushTask(client *caldav.Client, database *store.Store, localTask task.Task, result *Result) error {
	var etag string
	var err error
	if !localTask.Deleted {
		etag, err = client.PutTask(localTask.Path, localTask.RawICalendar, localTask.ETag)
	} else if localTask.ETag != "" {
		err = client.Delete(localTask.Path, localTask.ETag)
	}
	switch {
	case errors.Is(err, caldav.ErrConflict):
		return markConflict(client, database, localTask, result)
	case err != nil:
		return err
	case localTask.Deleted:
		result.Deleted++
		return database.Remove(localTask.Path, localTask.Revision)
	case etag == "":
		if _, etag, err = client.GetTask(localTask.Path); err != nil {
			return err
		}
	}
	result.Pushed++
	return database.MarkPushed(localTask.Path, localTask.Revision, etag)
}

func markConflict(client *caldav.Client, database *store.Store, localTask task.Task, result *Result) error {
	data, etag, err := client.GetTask(localTask.Path)
	if errors.Is(err, caldav.ErrNotFound) && localTask.Deleted {
		return database.Remove(localTask.Path, localTask.Revision)
	}
	if err != nil && !errors.Is(err, caldav.ErrNotFound) {
		return err
	}
	// an earlier upload whose reply got lost (network drop, quitting mid-sync) left the same content on the server
	if err == nil && !localTask.Deleted && sameContent(localTask.RawICalendar, data) {
		result.Pushed++
		return database.MarkPushed(localTask.Path, localTask.Revision, etag)
	}
	return conflict(database, localTask.Path, data, etag, result)
}

// conflict stores the server's version next to the local one; empty data means the server deleted the task.
func conflict(database *store.Store, path, data, etag string, result *Result) error {
	result.Conflicts++
	return database.SetConflict(path, data, etag)
}

// fromRemote keeps malformed data verbatim with a warning instead of failing the sync.
func fromRemote(item caldav.RemoteTask, calendarPath string, result *Result) task.Task {
	restored, err := task.Restore(task.Sync{Path: item.Path, CalendarPath: calendarPath, ETag: item.ETag}, item.Data)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", path.Base(item.Path), err))
	}
	return restored
}

// sameContent ignores line order and folding, which servers like Radicale change when they store a task.
func sameContent(first, second string) bool {
	lines := func(data string) []string {
		var kept []string
		for _, line := range ical.Unfold(data) {
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
		slices.Sort(kept)
		return kept
	}
	return slices.Equal(lines(first), lines(second))
}

func anyDirty(tasks []task.Task) bool {
	for _, current := range tasks {
		if current.Dirty || current.Conflict {
			return true
		}
	}
	return false
}
