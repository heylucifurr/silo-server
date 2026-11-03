package requests

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeDeleter records the targets the delete path hands to the download
// service, and can fail on demand.
type fakeDeleter struct {
	mu      sync.Mutex
	calls   []Target
	err     error
	missing map[int64]bool
}

func (d *fakeDeleter) DeleteTarget(_ context.Context, _ Integration, target Target, _ DeleteOptions) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.calls = append(d.calls, target)
	return nil
}

func (d *fakeDeleter) TargetExists(_ context.Context, _ Integration, target Target) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return false, d.err
	}
	return !d.missing[target.ID], nil
}

func (d *fakeDeleter) qualities() []Quality {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Quality, 0, len(d.calls))
	for _, t := range d.calls {
		out = append(out, t.Quality)
	}
	return out
}

// seedDeleteRequest stores a queued active request with the given targets,
// assigning target ids the way CreateTarget does.
func seedDeleteRequest(store *fakeStore, id string, targets ...Target) *Request {
	if store.targets == nil {
		store.targets = map[string][]Target{}
	}
	req := &Request{ID: id, MediaType: MediaTypeMovie, TMDBID: 550, Status: StatusQueued, Outcome: OutcomeActive}
	store.requests[id] = req
	for i := range targets {
		targets[i].RequestID = id
		store.targetSeq++
		targets[i].ID = store.targetSeq
		store.targets[id] = append(store.targets[id], targets[i])
	}
	return req
}

func submittedTarget(quality Quality, connection string) Target {
	return Target{Quality: quality, IntegrationID: connection, IntegrationKind: "seerr", ExternalID: "ext-" + string(quality), Status: StatusQueued}
}

func TestDeleteRequestRemovesEveryTargetAndSoftRemoves(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"), submittedTarget(Quality2160p, "seerr"))
	deleter := &fakeDeleter{}
	svc := newTestService(store)
	svc.SetTargetService(deleter)

	got, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{RemoveMedia: true})
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if got.Outcome != OutcomeRemoved || got.State() != StateRemoved {
		t.Fatalf("request = %s/%s, want removed", got.Status, got.Outcome)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	if len(targets) != 0 {
		t.Fatalf("targets = %+v, want none", targets)
	}
	if q := deleter.qualities(); len(q) != 2 {
		t.Fatalf("downstream calls = %v, want both qualities", q)
	}
	if events := store.events["r1"]; len(events) != 2 {
		t.Fatalf("events = %+v, want a delete event per target", events)
	}
}

func TestDeleteRequestKeepsRemainingTarget(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"), submittedTarget(Quality2160p, "seerr"))
	svc := newTestService(store)
	svc.SetTargetService(&fakeDeleter{})

	got, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{Qualities: []Quality{Quality1080p}})
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if got.Outcome != OutcomeActive {
		t.Fatalf("outcome = %s, want active while the 4K target remains", got.Outcome)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	if len(targets) != 1 || targets[0].Quality != Quality2160p {
		t.Fatalf("targets = %+v, want only the 4K target", targets)
	}
}

func TestDeleteRequestDownstreamErrorKeepsTheRow(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	svc := newTestService(store)
	svc.SetTargetService(&fakeDeleter{err: errors.New("seerr unreachable")})

	_, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{})
	if err == nil {
		t.Fatal("error = nil, want the downstream failure")
	}
	req := store.requests["r1"]
	if req.Outcome != OutcomeActive {
		t.Fatalf("outcome = %s, want active (the Silo record is untouched)", req.Outcome)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want the target kept", targets)
	}
}

func TestDeleteRequestNonAdminForbidden(t *testing.T) {
	store := newFakeStore()
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	svc := newTestService(store)

	_, err := svc.DeleteRequest(context.Background(), testViewer(1), "r1", DeleteOptions{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestDeleteRequestPendingWithNoTargets(t *testing.T) {
	store := newFakeStore()
	store.requests["r1"] = &Request{ID: "r1", MediaType: MediaTypeMovie, TMDBID: 550, Status: StatusPending, Outcome: OutcomeActive}
	svc := newTestService(store)

	got, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{})
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if got.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", got.Outcome)
	}
}

func TestDeleteRequestIsIdempotent(t *testing.T) {
	store := newFakeStore()
	store.requests["r1"] = &Request{ID: "r1", MediaType: MediaTypeMovie, TMDBID: 550, Status: StatusQueued, Outcome: OutcomeRemoved}
	svc := newTestService(store)

	got, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{})
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if got.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", got.Outcome)
	}
}

func TestDeleteRequestSiloOnlySkipsDownstream(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	deleter := &fakeDeleter{}
	svc := newTestService(store)
	svc.SetTargetService(deleter)

	got, err := svc.DeleteRequest(context.Background(), Viewer{UserID: 1, IsAdmin: true}, "r1", DeleteOptions{SiloOnly: true})
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if got.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", got.Outcome)
	}
	if len(deleter.calls) != 0 {
		t.Fatalf("downstream calls = %v, want none for Silo-only", deleter.qualities())
	}
}

// A request deleted in Seerr directly is retired by the reconcile pass instead
// of sitting in Processing forever.
func TestReconcileRetiresTargetsDeletedInSeerr(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}
	deleter := &fakeDeleter{missing: map[int64]bool{1: true}}
	svc := newTestService(store)
	svc.SetRouterProvider(&fakeRouterProvider{})
	svc.SetTargetService(deleter)

	result, err := svc.ReconcileRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("ReconcileRequests: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("removed = %d, want 1", result.Removed)
	}
	if got := store.requests["r1"]; got.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", got.Outcome)
	}
}

// A Seerr lookup that fails leaves the request alone: a dependency failure
// must not silently delete it.
func TestReconcileKeepsTargetsWhenSeerrIsUnreachable(t *testing.T) {
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}
	svc := newTestService(store)
	svc.SetRouterProvider(&fakeRouterProvider{})
	svc.SetTargetService(&fakeDeleter{err: errors.New("seerr unreachable")})

	if _, err := svc.ReconcileRequests(context.Background(), 10); err != nil {
		t.Fatalf("ReconcileRequests: %v", err)
	}
	req := store.requests["r1"]
	if req.Outcome != OutcomeActive {
		t.Fatalf("outcome = %s, want active (Seerr was unreachable)", req.Outcome)
	}
	if targets, _ := store.ListTargets(context.Background(), "r1"); len(targets) != 1 {
		t.Fatalf("targets = %+v, want the target kept", targets)
	}
}
