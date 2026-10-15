package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/puhi8/tasks/internal/syncer"
)

const pushDelay = 10 * time.Second

type autoSyncDue struct{}

type pushDue struct{}

type syncDone struct {
	result syncer.Result
	err    error
}

func (model *Model) scheduleAutoSync() tea.Cmd {
	if model.client == nil || model.syncInterval <= 0 {
		return nil
	}
	return tea.Tick(model.syncInterval, func(time.Time) tea.Msg { return autoSyncDue{} })
}

// schedulePush leaves changes that could not be uploaded (server unreachable or refusing them) to the regular sync,
// so they do not trigger a new sync every few seconds.
func (model *Model) schedulePush() tea.Cmd {
	if model.client == nil || model.syncInterval <= 0 || model.pending == 0 || model.pending == model.stuckPending ||
		model.pushScheduled || model.syncing || model.syncFailed {
		return nil
	}
	model.pushScheduled = true
	return tea.Tick(pushDelay, func(time.Time) tea.Msg { return pushDue{} })
}

func (model *Model) startSync() tea.Cmd {
	if model.client == nil {
		model.message = "local only: add a [server] section to the config to sync"
		if model.syncOff != "" {
			model.message = model.syncOff
		}
		return nil
	}
	if model.syncing {
		return nil
	}
	model.syncing = true
	client, database := model.client, model.database
	return func() tea.Msg {
		result, err := syncer.Run(client, database)
		return syncDone{result: result, err: err}
	}
}

func (model *Model) finishSync(done syncDone) {
	model.syncing = false
	model.syncFailed = done.err != nil
	model.reload()
	model.stuckPending = 0
	if done.result.Pushed+done.result.Deleted == 0 {
		model.stuckPending = model.pending
	}
	if done.err != nil {
		model.syncStatus = "offline ✗"
		model.message = "sync failed: " + strings.SplitN(done.err.Error(), "\n", 2)[0]
		return
	}
	model.syncStatus = "synced ✓ " + time.Now().Format("15:04")
	result := done.result
	model.message = ""
	if result.Pulled+result.Pushed+result.Deleted > 0 {
		model.message = fmt.Sprintf("↓%d ↑%d ✗%d", result.Pulled, result.Pushed, result.Deleted)
	}
	if result.Conflicts > 0 {
		model.message += fmt.Sprintf("  %d conflict(s): open the task marked ! to resolve", result.Conflicts)
	}
	if len(result.Warnings) > 0 {
		model.message += fmt.Sprintf("  warning: %s (+%d more)", result.Warnings[0], len(result.Warnings)-1)
	}
}
