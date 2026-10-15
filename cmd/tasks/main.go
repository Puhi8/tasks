package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/puhi8/tasks/internal/caldav"
	"github.com/puhi8/tasks/internal/config"
	"github.com/puhi8/tasks/internal/store"
	"github.com/puhi8/tasks/internal/syncer"
	"github.com/puhi8/tasks/internal/tui"
)

//go:embed usage.txt
var usage string

// version is set by release builds: go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	configPath := flag.String("config", config.DefaultPath(), "config file")
	listName := flag.String("list", "", "open the list with this name (default: all lists)")
	syncOnly := flag.Bool("sync", false, "sync once and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		if settings, err := config.Load(*configPath); err == nil {
			tui.SetAccent(settings.UI.Accent)
		}
		fmt.Fprintln(flag.CommandLine.Output(), tui.Markup(usage))
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if err := run(*configPath, *listName, *syncOnly); err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
}

func run(configPath, listName string, syncOnly bool) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	tui.SetAccent(settings.UI.Accent)
	database, err := store.Open(settings.Storage.Database)
	if err != nil {
		return err
	}
	defer database.Close()

	var client *caldav.Client
	var syncOff error
	if settings.Server.URL != "" {
		password, err := settings.Password()
		if err == nil {
			client, err = caldav.New(settings.Server.URL, settings.Server.Username, password)
		}
		// a locked keyring or a bad url must not lock you out of your tasks: the app opens without syncing
		if err != nil && syncOnly {
			return err
		}
		syncOff = err
	} else if err := database.EnsureLocalCalendar(); err != nil {
		return err
	}

	if syncOnly {
		if client == nil {
			return errors.New("no server configured in " + configPath)
		}
		return syncAndReport(client, database)
	}
	if _, err := tea.NewProgram(tui.New(database, client, settings, listName, syncOff), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		return err
	}
	if client == nil || !settings.Sync.SyncOnExit {
		return nil
	}
	if dirty, err := database.HasDirty(); err != nil || !dirty {
		return err
	}
	fmt.Println("syncing local changes… (ctrl+c skips, they are uploaded next time)")
	return syncAndReport(client, database)
}

func syncAndReport(client *caldav.Client, database *store.Store) error {
	result, err := syncer.Run(client, database)
	// a sync that failed before doing anything should not look like it ran
	if err == nil || result.Pulled+result.Pushed+result.Deleted+result.Conflicts > 0 {
		fmt.Printf("pulled %d, pushed %d, deleted %d, conflicts %d\n", result.Pulled, result.Pushed, result.Deleted, result.Conflicts)
	}
	for _, warning := range result.Warnings {
		fmt.Println("warning:", warning)
	}
	return err
}
