package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/puhi8/tasks/internal/task"
)

// LocalCalendarPath holds tasks when no server is configured; sync never pushes it.
const LocalCalendarPath = "/local/"

const taskTable = `
	path          TEXT PRIMARY KEY,
	calendar_path TEXT NOT NULL REFERENCES calendars(path) ON DELETE CASCADE,
	etag          TEXT NOT NULL DEFAULT '',
	raw_ical      TEXT NOT NULL,
	dirty         INTEGER NOT NULL DEFAULT 0,
	deleted       INTEGER NOT NULL DEFAULT 0,
	revision      INTEGER NOT NULL DEFAULT 0,
	conflict      INTEGER NOT NULL DEFAULT 0,
	remote_ical   TEXT NOT NULL DEFAULT '',
	remote_etag   TEXT NOT NULL DEFAULT ''
`

var migrations = []string{`
CREATE TABLE calendars (
	path         TEXT PRIMARY KEY,
	display_name TEXT NOT NULL DEFAULT '',
	ctag         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE tasks (` + taskTable + `);
`, `
CREATE TABLE tag_colors (
	tag   TEXT PRIMARY KEY,
	color TEXT NOT NULL,
	dirty INTEGER NOT NULL DEFAULT 0
);
`, `
ALTER TABLE calendars ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0;
`, `
CREATE TABLE collapsed (uid TEXT PRIMARY KEY);
`, `
-- tasks are read from raw_ical; older databases also kept every field in its own column
CREATE TABLE slim_tasks (` + taskTable + `);
INSERT INTO slim_tasks SELECT path, calendar_path, etag, raw_ical, dirty, deleted, revision, conflict, remote_ical, remote_etag FROM tasks;
DROP TABLE tasks;
ALTER TABLE slim_tasks RENAME TO tasks;
CREATE INDEX tasks_calendar ON tasks(calendar_path);
`}

type Store struct{ database *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// one connection serializes the TUI and the background sync
	database.SetMaxOpenConns(1)
	if err := migrate(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrating %s: %w", path, err)
	}
	return &Store{database: database}, nil
}

func (store *Store) Close() error { return store.database.Close() }

func migrate(database *sql.DB) error {
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for index := version; index < len(migrations); index++ {
		transaction, err := database.Begin()
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(migrations[index] + fmt.Sprintf(";PRAGMA user_version = %d", index+1)); err != nil {
			transaction.Rollback()
			return err
		}
		if err := transaction.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Calendars returns the lists that are not waiting to be deleted.
func (store *Store) Calendars() ([]task.List, error) {
	return collect(store, func(rows *sql.Rows) (calendar task.List, err error) {
		err = rows.Scan(&calendar.Path, &calendar.Name, &calendar.CTag)
		return
	}, "SELECT path, display_name, ctag FROM calendars WHERE deleted = 0 ORDER BY display_name COLLATE NOCASE")
}

func (store *Store) SaveCalendar(calendar task.List) error {
	return store.exec(`INSERT INTO calendars (path, display_name, ctag) VALUES (?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET display_name = excluded.display_name, ctag = excluded.ctag`, calendar.Path, calendar.Name, calendar.CTag)
}

func (store *Store) DeleteCalendar(path string) error {
	return store.exec("DELETE FROM calendars WHERE path = ?", path)
}

// MarkCalendarDeleted hides a list until sync has deleted it on the server.
func (store *Store) MarkCalendarDeleted(path string) error {
	return store.exec("UPDATE calendars SET deleted = 1 WHERE path = ?", path)
}

func (store *Store) DeletedCalendars() ([]string, error) {
	return store.column("SELECT path FROM calendars WHERE deleted = 1")
}

func (store *Store) column(query string, arguments ...any) ([]string, error) {
	return collect(store, func(rows *sql.Rows) (value string, err error) {
		err = rows.Scan(&value)
		return
	}, query, arguments...)
}

// collect runs a query and turns every row into an item with scan; methods cannot be generic, so store is a parameter.
func collect[Item any](store *Store, scan func(*sql.Rows) (Item, error), query string, arguments ...any) ([]Item, error) {
	rows, err := store.database.Query(query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (store *Store) EnsureLocalCalendar() error {
	return store.exec("INSERT OR IGNORE INTO calendars (path, display_name) VALUES (?, 'Local')", LocalCalendarPath)
}

func (store *Store) exec(query string, arguments ...any) error {
	_, err := store.database.Exec(query, arguments...)
	return err
}
