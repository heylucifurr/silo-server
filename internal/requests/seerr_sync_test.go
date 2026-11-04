package requests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeSeerrTarget implements both TargetService (so the service wires it as the
// download-service client) and SeerrStatusReader (so seerrStatusReader finds it
// through the same field). statuses maps a Seerr request id to the status Seerr
// reports; a nil value means Seerr answered 404. errs makes a lookup fail the
// way an offline Seerr does. lookups records every id asked about.
type fakeSeerrTarget struct {
	mu       sync.Mutex
	statuses map[string]*SeerrRequestStatus
	errs     map[string]error
	lookups  []string
}

func (f *fakeSeerrTarget) RequestStatus(_ context.Context, _ Integration, externalID string) (*SeerrRequestStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, externalID)
	if err := f.errs[externalID]; err != nil {
		return nil, err
	}
	status, ok := f.statuses[externalID]
	if !ok {
		return nil, errors.New("unexpected seerr lookup for " + externalID)
	}
	return status, nil
}

func (f *fakeSeerrTarget) DeleteTarget(context.Context, Integration, Target, DeleteOptions) error {
	return nil
}

func (f *fakeSeerrTarget) TargetExists(context.Context, Integration, Target) (bool, error) {
	return true, nil
}

func (f *fakeSeerrTarget) lookedUp() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lookups...)
}

// TestSeerrTargetStatusMapping pins the Seerr -> Silo table, including which
// media field each quality reads.
func TestSeerrTargetStatusMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		seerr  *SeerrRequestStatus
		target Target
		want   Status
		change bool
	}{
		{
			name:   "nil status leaves the target",
			seerr:  nil,
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   "",
			change: false,
		},
		{
			name:   "request not found leaves the target",
			seerr:  &SeerrRequestStatus{Found: false},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   "",
			change: false,
		},
		{
			name:   "HD available completes a queued HD target",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaAvailable},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   StatusCompleted,
			change: true,
		},
		{
			name:   "HD available leaves a completed target alone",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaAvailable},
			target: Target{Quality: Quality1080p, Status: StatusCompleted},
			want:   "",
			change: false,
		},
		{
			name:   "4K target reads status4k, not status",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaAvailable, MediaStatus4K: seerrMediaProcessing},
			target: Target{Quality: Quality2160p, Status: StatusQueued},
			want:   StatusDownloading,
			change: true,
		},
		{
			name:   "HD target reads status, not status4k",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaProcessing, MediaStatus4K: seerrMediaAvailable},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   StatusDownloading,
			change: true,
		},
		{
			name:   "partially available moves a queued target to downloading",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaPartial},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   StatusDownloading,
			change: true,
		},
		{
			name:   "partially available moves an approved target to downloading",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaPartial},
			target: Target{Quality: Quality1080p, Status: StatusApproved},
			want:   StatusDownloading,
			change: true,
		},
		{
			name:   "partially available leaves an already downloading target",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaPartial},
			target: Target{Quality: Quality1080p, Status: StatusDownloading},
			want:   "",
			change: false,
		},
		{
			name:   "processing moves a queued target to downloading",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaProcessing},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   StatusDownloading,
			change: true,
		},
		{
			name:   "processing leaves an already downloading target",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaProcessing},
			target: Target{Quality: Quality1080p, Status: StatusDownloading},
			want:   "",
			change: false,
		},
		{
			name:   "processing leaves a failed target to the admin",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaProcessing},
			target: Target{Quality: Quality1080p, Status: StatusFailed},
			want:   "",
			change: false,
		},
		{
			name:   "available completes a failed target",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaAvailable},
			target: Target{Quality: Quality1080p, Status: StatusFailed},
			want:   StatusCompleted,
			change: true,
		},
		{
			name:   "pending leaves the target as it is",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaPending},
			target: Target{Quality: Quality1080p, Status: StatusApproved},
			want:   "",
			change: false,
		},
		{
			name:   "unknown media status leaves the target",
			seerr:  &SeerrRequestStatus{Found: true, MediaStatus: seerrMediaUnknown},
			target: Target{Quality: Quality1080p, Status: StatusQueued},
			want:   "",
			change: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, changed := seerrTargetStatus(tc.seerr, tc.target)
			if got != tc.want || changed != tc.change {
				t.Fatalf("seerrTargetStatus = (%q, %v), want (%q, %v)", got, changed, tc.want, tc.change)
			}
		})
	}
}

// TestSyncSeerrRequestsCompletesAvailableTitle: an HD target queued in Silo
// whose Seerr media is available becomes Completed, and the request closes as
// Completed with a history event.
func TestSyncSeerrRequestsCompletesAvailableTitle(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable},
		}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Checked != 1 || result.Completed != 1 || result.Errors != 0 {
		t.Fatalf("result = %+v, want 1 checked, 1 completed, 0 errors", result)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	if len(targets) != 1 || targets[0].Status != StatusCompleted {
		t.Fatalf("targets = %+v, want one completed target", targets)
	}
	req := store.requests["r1"]
	if req.Status != StatusCompleted || req.Outcome != OutcomeActive {
		t.Fatalf("request = %s/%s, want completed/active", req.Status, req.Outcome)
	}
	events, _ := store.ListEvents(context.Background(), "r1", 10)
	if len(events) != 1 || events[0].EventType != "synced_from_seerr" {
		t.Fatalf("events = %+v, want one synced_from_seerr event", events)
	}
	if events[0].Message != "Synced from Seerr: 1080p is available" {
		t.Fatalf("event message = %q", events[0].Message)
	}
}

// TestSyncSeerrRequestsDeclinedClosesRequest: a request declined in Seerr closes
// as Declined, and the request's targets are left as they are.
func TestSyncSeerrRequestsDeclinedClosesRequest(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, Declined: true},
		}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Declined != 1 {
		t.Fatalf("declined = %d, want 1", result.Declined)
	}
	req := store.requests["r1"]
	if req.Outcome != OutcomeDeclined {
		t.Fatalf("outcome = %s, want declined", req.Outcome)
	}
	// Closing records the reason on the request; the repository writes the
	// request's history line for it, so the sync records no second one.
	if req.OutcomeReason != seerrDeclinedMessage {
		t.Fatalf("outcome reason = %q, want %q", req.OutcomeReason, seerrDeclinedMessage)
	}
	if targets, _ := store.ListTargets(context.Background(), "r1"); len(targets) != 1 {
		t.Fatalf("targets = %+v, want the target kept", targets)
	}
}

// TestSyncSeerrRequestsRemovesMissingRequest: Seerr answers 404 for the request,
// so its targets are removed and the request closes as Removed, keeping history.
func TestSyncSeerrRequestsRemovesMissingRequest(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{"ext-1080p": nil}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("removed = %d, want 1", result.Removed)
	}
	if targets, _ := store.ListTargets(context.Background(), "r1"); len(targets) != 0 {
		t.Fatalf("targets = %+v, want none after 404", targets)
	}
	req := store.requests["r1"]
	if req.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", req.Outcome)
	}
	// The retirement records why in the request's history, which is kept: the
	// closing write's own entry, then the marker naming where the closure came
	// from — newest, which is what tells a later run a retirement was left
	// unfinished.
	events, _ := store.ListEvents(context.Background(), "r1", 10)
	if len(events) != 2 || events[0].EventType != "outcome_removed" {
		t.Fatalf("events = %+v, want the close then the retirement marker", events)
	}
	if events[1].EventType != seerrRetiredEvent || events[1].Message != seerrRemovedMessage {
		t.Fatalf("events = %+v, want %s last with the reason", events, seerrRetiredEvent)
	}
}

// TestSyncSeerrRequestsNeverLeavesARequestSubmittable: a request Seerr no longer
// has must not end up approved and active with no targets, which is the state
// the reconcile pass submits — that is how a title deleted in Seerr came back as
// a new request. After the sync the request is closed, so the pass that follows
// has nothing to submit.
func TestSyncSeerrRequestsNeverLeavesARequestSubmittable(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{"ext-1080p": nil}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}
	router := &fakeRouterProvider{}
	svc.SetRouterProvider(router)

	if _, err := svc.SyncSeerrRequests(context.Background(), 10); err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}

	// The fake lists the seeded candidates verbatim, so the pass walks this
	// request as the repository would after its retirement: it must refuse to
	// submit a request that is no longer active, and ask the router for nothing.
	if _, err := svc.ReconcileRequests(context.Background(), 100); err != nil {
		t.Fatalf("ReconcileRequests: %v", err)
	}
	if router.fulfillCalls != 0 {
		t.Fatalf("router fulfill calls = %d, want 0: a title Seerr dropped must not be requested again", router.fulfillCalls)
	}
	if targets, _ := store.ListTargets(context.Background(), "r1"); len(targets) != 0 {
		t.Fatalf("targets = %+v, want none: nothing was sent again", targets)
	}
}

// TestSyncSeerrRequestsClosesARetirementThatDidNotFinish: a request left active
// with no targets whose newest history entry is the Seerr retirement — an older
// build's half-finished removal — is closed by the next run instead of being
// submitted again.
func TestSyncSeerrRequestsClosesARetirementThatDidNotFinish(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t, nil, nil)
	seedDeleteRequest(store, "r1")
	store.requests["r1"].Status = StatusApproved
	if err := store.RecordEvent(context.Background(), "r1", "removed_in_seerr", Viewer{}, seerrRemovedMessage); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("result = %+v, want 1 removed", result)
	}
	if req := store.requests["r1"]; req.Outcome != OutcomeRemoved {
		t.Fatalf("outcome = %s, want removed", req.Outcome)
	}
}

// TestSyncSeerrRequestsLeavesAFreshApprovalAlone: the same shape — active, no
// targets — with an approval behind it is a request waiting to be submitted, and
// the sync must not close it.
func TestSyncSeerrRequestsLeavesAFreshApprovalAlone(t *testing.T) {
	t.Parallel()
	svc, store, fake := newSeerrSyncService(t, nil, nil)
	seedDeleteRequest(store, "r1")
	store.requests["r1"].Status = StatusApproved
	if err := store.RecordEvent(context.Background(), "r1", "created", Viewer{}, ""); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Skipped != 1 || result.Removed != 0 {
		t.Fatalf("result = %+v, want 1 skipped and none removed", result)
	}
	if req := store.requests["r1"]; req.Outcome != OutcomeActive || req.Status != StatusApproved {
		t.Fatalf("request = %s/%s, want it left for the reconcile pass", req.Status, req.Outcome)
	}
	if got := len(fake.lookedUp()); got != 0 {
		t.Fatalf("lookups = %d, want none: nothing to check without a target", got)
	}
}

// TestSyncSeerrRequestsScansWhenSeerrReportsMedia: completing a target from
// Seerr's available status asks for a scan of the libraries that can hold the
// title. A run that completed nothing asks for none.
func TestSyncSeerrRequestsScansWhenSeerrReportsMedia(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable}}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}
	scanner := &fakeLibraryScanner{}
	svc.SetLibraryScanner(scanner)

	if _, err := svc.SyncSeerrRequests(context.Background(), 10); err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if len(scanner.scanned) != 1 || scanner.scanned[0] != MediaTypeMovie {
		t.Fatalf("scanned = %v, want the movie libraries once", scanner.scanned)
	}

	// The target is completed now, so a second run changes nothing and asks for
	// no further scan.
	if _, err := svc.SyncSeerrRequests(context.Background(), 10); err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if len(scanner.scanned) != 1 {
		t.Fatalf("scanned = %v, want no second scan", scanner.scanned)
	}
}

// fakeLibraryScanner records the media types a scan was asked for.
type fakeLibraryScanner struct {
	scanned []MediaType
	err     error
}

func (f *fakeLibraryScanner) ScanLibrariesFor(_ context.Context, mediaType MediaType) error {
	f.scanned = append(f.scanned, mediaType)
	return f.err
}

// TestSyncSeerrRequestsKeepsRequestWhenSeerrIsOffline: a lookup failure counts
// in Errors and leaves the request untouched, so the next run retries instead of
// reading an outage as a deletion.
func TestSyncSeerrRequestsKeepsRequestWhenSeerrIsOffline(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t, nil,
		map[string]error{"ext-1080p": errors.New("seerr unreachable")})
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err == nil {
		t.Fatal("SyncSeerrRequests returned no error; the first failure should surface")
	}
	if !errors.Is(err, ErrIntegrationUnreachable) {
		t.Fatalf("err = %v, want ErrIntegrationUnreachable", err)
	}
	if result.Errors != 1 || result.Removed != 0 {
		t.Fatalf("result = %+v, want 1 error and 0 removed", result)
	}
	req := store.requests["r1"]
	if req.Outcome != OutcomeActive {
		t.Fatalf("outcome = %s, want active (Seerr was unreachable)", req.Outcome)
	}
	if targets, _ := store.ListTargets(context.Background(), "r1"); len(targets) != 1 {
		t.Fatalf("targets = %+v, want the target kept", targets)
	}
}

// TestSyncSeerrRequestsReadsOneLookupForSharedHDAnd4K: Seerr keeps one request
// for a title's HD and 4K variants. Both targets must cost one lookup and read
// their own media field: HD from status, 4K from status4k.
func TestSyncSeerrRequestsReadsOneLookupForSharedHDAnd4K(t *testing.T) {
	t.Parallel()
	svc, store, fake := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-shared": {Found: true, MediaStatus: seerrMediaAvailable, MediaStatus4K: seerrMediaProcessing},
		}, nil)
	hd := Target{Quality: Quality1080p, IntegrationID: "seerr", IntegrationKind: "seerr", ExternalID: "ext-shared", Status: StatusQueued}
	uhd := Target{Quality: Quality2160p, IntegrationID: "seerr", IntegrationKind: "seerr", ExternalID: "ext-shared", Status: StatusQueued}
	seedDeleteRequest(store, "r1", hd, uhd)
	store.candidates = []*Request{store.requests["r1"]}

	if _, err := svc.SyncSeerrRequests(context.Background(), 10); err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if got := len(fake.lookedUp()); got != 1 {
		t.Fatalf("lookups = %d, want 1 for the shared request", got)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	byQuality := map[Quality]Status{}
	for _, target := range targets {
		byQuality[target.Quality] = target.Status
	}
	if byQuality[Quality1080p] != StatusCompleted {
		t.Fatalf("HD target = %s, want completed (status=available)", byQuality[Quality1080p])
	}
	if byQuality[Quality2160p] != StatusDownloading {
		t.Fatalf("4K target = %s, want downloading (status4k=processing)", byQuality[Quality2160p])
	}
}

// TestSyncSeerrRequestsMarksPartialDownloading: partially available media keeps
// the request in progress and moves a queued target to Downloading.
func TestSyncSeerrRequestsMarksPartialDownloading(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, MediaStatus: seerrMediaPartial},
		}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Downloading != 1 {
		t.Fatalf("downloading = %d, want 1", result.Downloading)
	}
	targets, _ := store.ListTargets(context.Background(), "r1")
	if len(targets) != 1 || targets[0].Status != StatusDownloading {
		t.Fatalf("targets = %+v, want one downloading target", targets)
	}
	if req := store.requests["r1"]; req.Outcome != OutcomeActive {
		t.Fatalf("outcome = %s, want active (partial stays in progress)", req.Outcome)
	}
}

// TestSyncOutcomeReportsOnlyATotalFailure pins when a run reports failure: a run
// that reached Seerr for at least one request completed with its failures
// counted, while a run Seerr answered nothing for fails, so its history shows
// the outage.
func TestSyncOutcomeReportsOnlyATotalFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("seerr unreachable")
	cases := []struct {
		name   string
		result SeerrSyncResult
		err    error
		want   error
	}{
		{name: "no failures", result: SeerrSyncResult{Checked: 3}, want: nil},
		{name: "nothing to check", result: SeerrSyncResult{}, err: boom, want: nil},
		{name: "one of three failed", result: SeerrSyncResult{Checked: 3, Errors: 1}, err: boom, want: nil},
		{name: "every request failed", result: SeerrSyncResult{Checked: 2, Errors: 2}, err: boom, want: boom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := syncOutcome(tc.result, tc.err); got != tc.want {
				t.Fatalf("syncOutcome = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSyncSeerrRequestsKeepsGoingAfterOneLookupFails: one request whose Seerr
// lookup fails counts in Errors and leaves that request alone, while the rest of
// the batch is still applied.
func TestSyncSeerrRequestsKeepsGoingAfterOneLookupFails(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable},
		},
		map[string]error{"ext-broken": errors.New("seerr answered 500")})
	ok := seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	broken := seedDeleteRequest(store, "r2", Target{
		Quality: Quality1080p, IntegrationID: "seerr", IntegrationKind: "seerr",
		ExternalID: "ext-broken", Status: StatusQueued,
	})
	store.candidates = []*Request{ok, broken}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Checked != 2 || result.Completed != 1 || result.Errors != 1 {
		t.Fatalf("result = %+v, want 2 checked, 1 completed, 1 error", result)
	}
	if req := store.requests["r1"]; req.Status != StatusCompleted {
		t.Fatalf("r1 status = %s, want completed", req.Status)
	}
	if req := store.requests["r2"]; req.Status != StatusQueued || req.Outcome != OutcomeActive {
		t.Fatalf("r2 = %s/%s, want queued/active (left as it was)", req.Status, req.Outcome)
	}
}

// TestSyncSeerrRequestsSkipsCompletedTargets: a request whose targets already
// completed costs no Seerr lookup and counts as skipped.
func TestSyncSeerrRequestsSkipsCompletedTargets(t *testing.T) {
	t.Parallel()
	svc, store, fake := newSeerrSyncService(t, nil, nil)
	done := submittedTarget(Quality1080p, "seerr")
	done.Status = StatusCompleted
	seedDeleteRequest(store, "r1", done)
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Skipped != 1 || result.Completed != 0 {
		t.Fatalf("result = %+v, want 1 skipped", result)
	}
	if got := len(fake.lookedUp()); got != 0 {
		t.Fatalf("lookups = %d, want 0 for completed targets", got)
	}
}

// TestSyncSeerrRequestsAllPagesThroughEveryRequest: the first-deploy sweep reads
// the admin list page by page, correcting active entries, leaving completed
// ones alone, and leaving requests a person closed untouched.
func TestSyncSeerrRequestsAllPagesThroughEveryRequest(t *testing.T) {
	t.Parallel()
	svc, store, fake := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable},
			"ext-decl":  {Found: true, Declined: true},
		}, nil)
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	r1 := seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	r1.CreatedAt = base
	r2 := seedDeleteRequest(store, "r2", Target{Quality: Quality1080p, IntegrationID: "seerr", IntegrationKind: "seerr", ExternalID: "ext-decl", Status: StatusQueued})
	r2.CreatedAt = base.Add(-time.Hour)
	r3 := seedDeleteRequest(store, "r3", Target{Quality: Quality1080p, IntegrationID: "seerr", IntegrationKind: "seerr", ExternalID: "ext-done", Status: StatusCompleted})
	r3.CreatedAt = base.Add(-2 * time.Hour)
	r4 := &Request{ID: "r4", MediaType: MediaTypeMovie, Status: StatusCompleted, Outcome: OutcomeActive}
	r4.CreatedAt = base.Add(-3 * time.Hour)
	store.requests["r4"] = r4
	// Requests a person closed keep the outcome they gave them, however Seerr
	// answers for the same Seerr id the sweep does read.
	r5 := seedDeleteRequest(store, "r5", submittedTarget(Quality1080p, "seerr"))
	r5.Outcome = OutcomeDeclined
	r5.CreatedAt = base.Add(-4 * time.Hour)
	r6 := seedDeleteRequest(store, "r6", submittedTarget(Quality1080p, "seerr"))
	r6.Outcome = OutcomeRemoved
	r6.CreatedAt = base.Add(-5 * time.Hour)
	// Newest first, matching the repository's order.
	store.adminList = []*Request{r1, r2, r3, r4, r5, r6}

	result, err := svc.SyncSeerrRequestsAll(context.Background(), 2)
	if err != nil {
		t.Fatalf("SyncSeerrRequestsAll: %v", err)
	}
	if result.Checked != 6 {
		t.Fatalf("checked = %d, want 6", result.Checked)
	}
	if result.Completed != 1 || result.Declined != 1 || result.Skipped != 4 {
		t.Fatalf("result = %+v, want 1 completed, 1 declined, 4 skipped", result)
	}
	if got := len(fake.lookedUp()); got != 2 {
		t.Fatalf("lookups = %d, want 2 (r1 and r2)", got)
	}
	if r5.Outcome != OutcomeDeclined || r6.Outcome != OutcomeRemoved {
		t.Fatalf("closed requests = %s/%s, want declined/removed", r5.Outcome, r6.Outcome)
	}
	// Three full pages of 2 plus the final empty page that confirms the end.
	if len(store.adminFilters) != 4 {
		t.Fatalf("admin pages read = %d, want 4", len(store.adminFilters))
	}
	if store.adminFilters[0].Before != nil {
		t.Fatal("first page should not carry a Before cursor")
	}
	if store.adminFilters[1].Before == nil {
		t.Fatal("second page should carry a Before cursor")
	}
	if store.adminFilters[0].Limit != 2 {
		t.Fatalf("limit = %d, want 2", store.adminFilters[0].Limit)
	}
}

// TestSyncSeerrRequestsWithoutConnectionDoesNothing: a server with no Seerr
// integration walks no requests and makes no lookups.
func TestSyncSeerrRequestsWithoutConnectionDoesNothing(t *testing.T) {
	t.Parallel()
	svc, store, fake := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable}}, nil)
	store.integrations = nil
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Checked != 0 {
		t.Fatalf("checked = %d, want 0 without a Seerr connection", result.Checked)
	}
	if got := len(fake.lookedUp()); got != 0 {
		t.Fatalf("lookups = %d, want 0 without a Seerr connection", got)
	}
}

// TestSyncSeerrRequestsWithoutReaderDoesNothing: a target service that cannot
// read status leaves the sync inert instead of failing.
func TestSyncSeerrRequestsWithoutReaderDoesNothing(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	store.candidates = []*Request{store.requests["r1"]}
	svc := newTestService(store)
	svc.SetTargetService(&fakeDeleter{})

	result, err := svc.SyncSeerrRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncSeerrRequests: %v", err)
	}
	if result.Checked != 0 {
		t.Fatalf("checked = %d, want 0 without a reader", result.Checked)
	}
}

// TestSyncSeerrRequestRefreshesOneByID: the admin's per-request refresh applies
// the same mapping and returns the request as stored.
func TestSyncSeerrRequestRefreshesOneByID(t *testing.T) {
	t.Parallel()
	svc, store, _ := newSeerrSyncService(t,
		map[string]*SeerrRequestStatus{
			"ext-1080p": {Found: true, MediaStatus: seerrMediaAvailable},
		}, nil)
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))

	got, err := svc.SyncSeerrRequest(context.Background(), "r1")
	if err != nil {
		t.Fatalf("SyncSeerrRequest: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
}

// TestSyncSeerrRequestReportsUnreachable: with no Seerr connection the refresh
// reports ErrIntegrationUnreachable so the API answers upstream-unavailable.
func TestSyncSeerrRequestReportsUnreachable(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	seedDeleteRequest(store, "r1", submittedTarget(Quality1080p, "seerr"))
	svc := newTestService(store)
	svc.SetTargetService(&fakeDeleter{})

	if _, err := svc.SyncSeerrRequest(context.Background(), "r1"); !errors.Is(err, ErrIntegrationUnreachable) {
		t.Fatalf("err = %v, want ErrIntegrationUnreachable", err)
	}
}

// TestSyncSeerrRequestUnknownIDReturnsNotFound: refreshing a request that does
// not exist surfaces ErrNotFound.
func TestSyncSeerrRequestUnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSeerrSyncService(t, nil, nil)
	if _, err := svc.SyncSeerrRequest(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// newSeerrSyncService seeds a store with a Seerr integration and returns the
// service with the fake reader wired as its target service.
func newSeerrSyncService(t *testing.T, statuses map[string]*SeerrRequestStatus, errs map[string]error) (*Service, *fakeStore, *fakeSeerrTarget) {
	t.Helper()
	store := newFakeStore()
	store.integrations = []Integration{seerrInst("seerr", true)}
	fake := &fakeSeerrTarget{statuses: statuses, errs: errs}
	svc := newTestService(store)
	svc.SetTargetService(fake)
	return svc, store, fake
}
