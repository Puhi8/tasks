package main

import (
	"embed"
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

//go:embed completion.bash completion.zsh
var completions embed.FS

// version is set by release builds: go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	configPath := flag.String("config", config.DefaultPath(), "config file")
	listName := flag.String("list", "", "the list to open or to work on (default: all lists)")
	due := flag.String("due", "", "due date of a task added with add")
	// --sync predates the sync command and stays for existing scripts
	syncOnly := flag.Bool("sync", false, "sync once and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		if settings, err := config.Load(*configPath); err == nil {
			tui.SetAccent(settings.UI.Accent)
		}
		fmt.Fprintln(flag.CommandLine.Output(), tui.Markup(usage))
	}
	words := parseArguments()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if len(words) > 0 && words[0] == "sync" {
		*syncOnly, words = true, nil
	}
	var err error
	if len(words) > 0 && words[0] == "completion" {
		err = printCompletion(words)
	} else {
		err = run(*configPath, *listName, *due, *syncOnly, words)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
}

func printCompletion(words []string) error {
	shell := ""
	if len(words) > 1 {
		shell = words[1]
	}
	script, err := completions.ReadFile("completion." + shell)
	if err != nil {
		return errors.New("completion needs a shell: bash or zsh")
	}
	fmt.Print(string(script))
	return nil
}

// parseArguments lets flags come after the command too, as in: tasks add Buy milk --due tomorrow
func parseArguments() []string {
	var words []string
	for arguments := os.Args[1:]; len(arguments) > 0; {
		flag.CommandLine.Parse(arguments)
		arguments = flag.Args()
		if len(arguments) > 0 {
			words = append(words, arguments[0])
			arguments = arguments[1:]
		}
	}
	return words
}

func run(configPath, listName, due string, syncOnly bool, words []string) error {
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
	if settings.Server.URL == "" {
		if err := database.EnsureLocalCalendar(); err != nil {
			return err
		}
	}

	if syncOnly {
		if settings.Server.URL == "" {
			return errors.New("no server configured in " + configPath)
		}
		client, err := connect(settings)
		if err != nil {
			return err
		}
		return syncAndReport(client, database)
	}
	if len(words) > 0 {
		changed, err := tui.Command(database, settings, listName, due, words)
		if err != nil || !changed || settings.Server.URL == "" || !settings.Sync.SyncOnExit {
			return err
		}
		client, err := connect(settings)
		if err == nil {
			err = syncAndReport(client, database)
		}
		if err != nil {
			return fmt.Errorf("saved here, uploaded on the next sync: %w", err)
		}
		return nil
	}
	// a locked keyring or a bad url must not lock you out of your tasks: the app opens without syncing
	client, syncOff := connect(settings)
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

// connect gives no client and no error when no server is configured.
func connect(settings config.Config) (*caldav.Client, error) {
	if settings.Server.URL == "" {
		return nil, nil
	}
	password, err := settings.Password()
	if err != nil {
		return nil, err
	}
	return caldav.New(settings.Server.URL, settings.Server.Username, password)
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
