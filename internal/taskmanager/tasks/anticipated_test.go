package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeAnticipatedRefresher struct {
	refreshed int
	err       error
	calls     int
}

func (f *fakeAnticipatedRefresher) Refresh(context.Context, int) (int, error) {
	f.calls++
	return f.refreshed, f.err
}

type noopProgress struct{}

func (noopProgress) Report(float64, string)        {}
func (noopProgress) SetResultData(json.RawMessage) {}

func TestRefreshAnticipatedTaskRuns(t *testing.T) {
	refresher := &fakeAnticipatedRefresher{refreshed: 2}
	task := NewRefreshAnticipatedTask(refresher)
	if err := task.Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if refresher.calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refresher.calls)
	}

	// Every entry failing is reported as a task failure.
	failing := &fakeAnticipatedRefresher{err: errors.New("boom")}
	if err := NewRefreshAnticipatedTask(failing).Execute(context.Background(), noopProgress{}); err == nil {
		t.Fatal("Execute: want an error when nothing refreshed")
	}

	// A partial success is not a task failure.
	partial := &fakeAnticipatedRefresher{refreshed: 1, err: errors.New("one failed")}
	if err := NewRefreshAnticipatedTask(partial).Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute partial: %v", err)
	}

	// No refresher configured is a no-op, not an error.
	if err := NewRefreshAnticipatedTask(nil).Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute nil: %v", err)
	}
}
