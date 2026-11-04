package tasks

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/Silo-Server/silo-server/internal/requests"
	"github.com/Silo-Server/silo-server/internal/taskmanager"
)

// SeerrRequestSyncer reads requests' status back from Seerr: the in-flight
// ones on every run, and every request on the one full pass a deployment owes
// when the sync first runs there.
type SeerrRequestSyncer interface {
	SyncSeerrRequests(ctx context.Context, limit int) (requests.SeerrSyncResult, error)
	SyncSeerrRequestsAll(ctx context.Context, limit int) (requests.SeerrSyncResult, error)
}

// SyncSeerrRequestsTask reconciles Silo's requests with the status Seerr reports
// for them: a request declined or deleted in Seerr closes in Silo, and a
// download Seerr has finished completes. It runs every 15 minutes by default,
// and an admin can run it on demand from the request queue.
//
// Its first run sweeps every request, Done and Failed included, so entries left
// stale before this sync existed are corrected; later runs cover the in-flight
// requests only. The caller decides whether that pass is still owed, from the
// task's own history.
type SyncSeerrRequestsTask struct {
	syncer SeerrRequestSyncer
	limit  int
	// sweepPending is cleared once the full pass completes, so a run that
	// fails or is cancelled leaves it owed.
	sweepPending atomic.Bool
}

// NewSyncSeerrRequestsTask constructs the task. A nil syncer makes the task a
// no-op, so a server without a Seerr connection still registers it.
// sweepOnFirstRun makes the first run cover every request, not just the
// in-flight ones.
func NewSyncSeerrRequestsTask(syncer SeerrRequestSyncer, limit int, sweepOnFirstRun bool) *SyncSeerrRequestsTask {
	if limit <= 0 {
		limit = 100
	}
	task := &SyncSeerrRequestsTask{syncer: syncer, limit: limit}
	task.sweepPending.Store(sweepOnFirstRun)
	return task
}

func (t *SyncSeerrRequestsTask) Key() string  { return "sync_seerr_requests" }
func (t *SyncSeerrRequestsTask) Name() string { return "Sync requests" }
func (t *SyncSeerrRequestsTask) Description() string {
	return "Reads each in-flight request's status back from Seerr, closing requests declined or deleted there and completing finished downloads"
}
func (t *SyncSeerrRequestsTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *SyncSeerrRequestsTask) IsHidden() bool { return false }

func (t *SyncSeerrRequestsTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 15 * 60 * 1000},
	}
}

func (t *SyncSeerrRequestsTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	if t.syncer == nil {
		progress.Report(100, "Seerr request sync unavailable")
		return nil
	}
	sweeping := t.sweepPending.Load()
	run, message := t.syncer.SyncSeerrRequests, "Syncing in-flight requests with Seerr"
	if sweeping {
		run, message = t.syncer.SyncSeerrRequestsAll, "Syncing every request with Seerr"
	}
	progress.Report(0, message)
	result, err := run(ctx, t.limit)
	if data, marshalErr := json.Marshal(result); marshalErr == nil {
		progress.SetResultData(data)
	}
	if err != nil {
		// A sweep that failed stays owed, so the next run tries the full pass
		// again instead of settling for the in-flight requests.
		return err
	}
	if sweeping {
		t.sweepPending.Store(false)
	}
	progress.Report(100, "Request status sync complete")
	return nil
}
