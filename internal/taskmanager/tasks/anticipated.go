package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/taskmanager"
)

// anticipatedRefreshPerRun bounds one run, so a long list cannot hold a task slot.
const anticipatedRefreshPerRun = 50

// AnticipatedRefresher re-reads the curated titles that are due for a check.
// Satisfied by *anticipated.Service.
type AnticipatedRefresher interface {
	Refresh(ctx context.Context, limit int) (int, error)
}

// RefreshAnticipatedTask keeps the curated "Most Anticipated" titles' metadata and
// trailers current, so the hero carries a fresh release date, artwork and clip
// without ever reading TMDB while a viewer waits.
type RefreshAnticipatedTask struct {
	refresher AnticipatedRefresher
}

func NewRefreshAnticipatedTask(refresher AnticipatedRefresher) *RefreshAnticipatedTask {
	return &RefreshAnticipatedTask{refresher: refresher}
}

func (t *RefreshAnticipatedTask) Key() string  { return "refresh_anticipated" }
func (t *RefreshAnticipatedTask) Name() string { return "Refresh Anticipated Titles" }
func (t *RefreshAnticipatedTask) Description() string {
	return "Re-reads the metadata and trailers of the Anticipated titles due for a check."
}
func (t *RefreshAnticipatedTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryMetadata
}
func (t *RefreshAnticipatedTask) IsHidden() bool { return false }
func (t *RefreshAnticipatedTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeStartup},
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: int64((6 * time.Hour) / time.Millisecond)},
	}
}

func (t *RefreshAnticipatedTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	if t.refresher == nil {
		progress.Report(100, "Anticipated refresh is not configured")
		return nil
	}
	refreshed, err := t.refresher.Refresh(ctx, anticipatedRefreshPerRun)
	if err != nil && refreshed == 0 {
		return fmt.Errorf("refresh anticipated titles: %w", err)
	}
	progress.Report(100, fmt.Sprintf("Refreshed %d anticipated titles", refreshed))
	return nil
}
