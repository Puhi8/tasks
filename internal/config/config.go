package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// PasswordVariable overrides password_command, so the password never has to be stored in the config file.
const PasswordVariable = "TASKS_TUI_PASSWORD"

type Config struct {
	Server struct {
		URL             string `toml:"url"`
		Username        string `toml:"username"`
		PasswordCommand string `toml:"password_command"`
	} `toml:"server"`
	Storage struct {
		Database string `toml:"database"`
	} `toml:"storage"`
	Sync struct {
		SyncOnStart bool `toml:"sync_on_start"`
		SyncOnExit  bool `toml:"sync_on_exit"`
		// IntervalMinutes also uploads local changes shortly after they are made; 0 turns both off.
		IntervalMinutes int `toml:"interval_minutes"`
	} `toml:"sync"`
	UI struct {
		Accent           string `toml:"accent"`
		ShowDescriptions bool   `toml:"show_descriptions"`
	} `toml:"ui"`
}

func DefaultPath() string {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(directory, "tasks-tui", "config.toml")
}

// Load reads the config file; a missing file gives the defaults (local-only, no server).
func Load(path string) (Config, error) {
	var loaded Config
	loaded.Sync.SyncOnStart, loaded.Sync.SyncOnExit, loaded.Sync.IntervalMinutes = true, true, 5
	if _, err := toml.DecodeFile(path, &loaded); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return loaded, fmt.Errorf("reading %s: %w", path, err)
	}
	if loaded.Storage.Database == "" {
		loaded.Storage.Database = defaultDatabase()
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(loaded.Storage.Database, "~/") {
		loaded.Storage.Database = filepath.Join(home, loaded.Storage.Database[2:])
	}
	return loaded, nil
}

func (settings Config) Password() (string, error) {
	if password, found := os.LookupEnv(PasswordVariable); found {
		return password, nil
	}
	if settings.Server.PasswordCommand == "" {
		return "", nil
	}
	output, err := exec.Command("sh", "-c", settings.Server.PasswordCommand).Output()
	var failed *exec.ExitError
	if errors.As(err, &failed) && len(failed.Stderr) > 0 {
		return "", fmt.Errorf("password_command failed: %s", strings.TrimSpace(string(failed.Stderr)))
	}
	if err != nil {
		return "", fmt.Errorf("password_command failed: %w", err)
	}
	return strings.TrimRight(string(output), "\r\n"), nil
}

func defaultDatabase() string {
	directory := os.Getenv("XDG_DATA_HOME")
	if directory == "" {
		home, _ := os.UserHomeDir()
		directory = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(directory, "tasks-tui", "tasks.db")
}
