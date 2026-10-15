package store

// Folded subtasks are a local view setting, keyed by UID so they survive moving a task between lists.

func (store *Store) Collapsed() (map[string]bool, error) {
	uids, err := store.column("SELECT uid FROM collapsed")
	collapsed := map[string]bool{}
	for _, uid := range uids {
		collapsed[uid] = true
	}
	return collapsed, err
}

func (store *Store) SetCollapsed(uid string, collapsed bool) error {
	if collapsed {
		return store.exec("INSERT OR IGNORE INTO collapsed (uid) VALUES (?)", uid)
	}
	return store.exec("DELETE FROM collapsed WHERE uid = ?", uid)
}
