package store

import (
	"database/sql"
	"strings"
)

// Tags are keyed in lower case; an empty color marks a reset that still has to reach the server.

func (store *Store) TagColors() (map[string]string, error) { return store.tagColors("color != ''") }

// DirtyTagColors returns the colors changed here since the last sync, "" meaning reset to automatic.
func (store *Store) DirtyTagColors() (map[string]string, error) { return store.tagColors("dirty = 1") }

func (store *Store) SetTagColor(tag, color string) error {
	return store.exec(`INSERT INTO tag_colors (tag, color, dirty) VALUES (?, ?, 1)
		ON CONFLICT(tag) DO UPDATE SET color = excluded.color, dirty = 1`, strings.ToLower(tag), color)
}

// ReplaceTagColors stores the server's colors, keeping edits made after pushed was read so the next sync sends them.
func (store *Store) ReplaceTagColors(remote, pushed map[string]string) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec("DELETE FROM tag_colors WHERE dirty = 0"); err != nil {
		return err
	}
	for tag, color := range pushed {
		if _, err := transaction.Exec("DELETE FROM tag_colors WHERE tag = ? AND color = ?", tag, color); err != nil {
			return err
		}
	}
	for tag, color := range remote {
		if _, err := transaction.Exec("INSERT OR IGNORE INTO tag_colors (tag, color, dirty) VALUES (?, ?, 0)", strings.ToLower(tag), color); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) tagColors(where string) (map[string]string, error) {
	pairs, err := collect(store, func(rows *sql.Rows) (pair [2]string, err error) {
		err = rows.Scan(&pair[0], &pair[1])
		return
	}, "SELECT tag, color FROM tag_colors WHERE "+where)
	colors := map[string]string{}
	for _, pair := range pairs {
		colors[pair[0]] = pair[1]
	}
	return colors, err
}
