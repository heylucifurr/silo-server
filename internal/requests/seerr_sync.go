package requests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// SeerrStatusReader is the part of the Seerr client the sync needs. A nil
// status means Seerr answered 404: the request no longer exists there. An
// error means the lookup failed and the request must be left as it is.
type SeerrStatusReader interface {
	RequestStatus(ctx context.Context, in Integration, externalID string) (*SeerrRequestStatus, error)
}

// SeerrSyncResult summarises one sync run over the requests it examined.
type SeerrSyncResult struct {
	Checked     int
	Declined    int
	Removed     int
	Completed   int
	Downloading int
	Skipped     int
	Errors      int
}

// seerrDeclinedMessage is the reason stored on a request closed because Seerr
// declined it.
const seerrDeclinedMessage = "Synced from Seerr: the request was declined in Seerr"

// seerrTargetStatus maps one Seerr media status to the Silo target status it
// implies. changed is false when the target already sits in that state, so the
// sync writes nothing for a request that has not moved.
//
// The mapping follows the request's quality: the 4K variant reads Seerr's
// status4k and the HD variant its status. Seerr's "partially available" stays
// in progress, so the request keeps waiting for the rest instead of completing
// early. "Unknown" and "pending" leave the target as it is: Seerr has not said
// enough to move it, and the download path owns the queued step.
//
// The sync only moves a target forward. A failed target is completed when
// Seerr reports the media available, since that is a fact about the library,
// but a download Seerr sees under way says nothing about why it failed, so
// that stays the admin's call to retry.
func seerrTargetStatus(seerr *SeerrRequestStatus, target Target) (Status, bool) {
	if seerr == nil || !seerr.Found {
		return "", false
	}
	media := seerr.MediaStatus
	if target.Quality == Quality2160p {
		media = seerr.MediaStatus4K
	}
	switch media {
	case seerrMediaAvailable:
		if target.Status == StatusCompleted {
			return "", false
		}
		return StatusCompleted, true
	case seerrMediaPartial, seerrMediaProcessing:
		if target.Status == StatusDownloading {
			return "", false
		}
		if target.Status == StatusFailed {
			return "", false
		}
		return StatusDownloading, true
	default:
		return "", false
	}
}

// seerrStatusPhrase names a status in a request's history line.
func seerrStatusPhrase(status Status) string {
	switch status {
	case StatusCompleted:
		return "is available"
	case StatusDownloading:
		return "is downloading"
	default:
		return "is " + string(status)
	}
}

// seerrStatusReader returns the Seerr client when the wired target service can
// read request status. A server without a Seerr connection, or with one that
// cannot read status, leaves every request as it is.
func (s *Service) seerrStatusReader() SeerrStatusReader {
	if s == nil || s.targets == nil {
		return nil
	}
	reader, _ := s.targets.(SeerrStatusReader)
	return reader
}

// hasSeerrConnection reports whether any enabled connection is a Seerr one. It
// keeps a server with no Seerr from walking every request for nothing.
func hasSeerrConnection(fc *fulfillContext) bool {
	if fc == nil {
		return false
	}
	for i := range fc.integrations {
		if fc.integrations[i].Enabled && fc.integrations[i].CapabilityID == seerrCapabilityID {
			return true
		}
	}
	return false
}

// SyncSeerrRequests reconciles active requests with the status Seerr reports
// for them. It walks the reconcile candidates in batches of limit and returns a
// summary. A per-request failure counts in Errors and does not stop the run; a
// Seerr lookup that fails leaves that request untouched so the next run retries
// instead of reading an outage as a deletion.
func (s *Service) SyncSeerrRequests(ctx context.Context, limit int) (SeerrSyncResult, error) {
	result := SeerrSyncResult{}
	if s == nil || s.store == nil {
		return result, fmt.Errorf("request service is not configured")
	}
	reader := s.seerrStatusReader()
	if reader == nil {
		return result, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	candidates, err := s.store.ListReconciliationCandidates(ctx, limit)
	if err != nil {
		return result, err
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return result, err
	}
	if !hasSeerrConnection(fc) {
		return result, nil
	}
	var firstErr error
	for _, req := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Checked++
		change, err := s.syncSeerrRequest(ctx, *req, fc, reader)
		if err != nil {
			result.Errors++
			slog.WarnContext(ctx, "seerr status sync failed", "component", "requests",
				"request_id", req.ID, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		switch change {
		case seerrChangeDeclined:
			result.Declined++
		case seerrChangeRemoved:
			result.Removed++
		case seerrChangeCompleted:
			result.Completed++
		case seerrChangeDownloading:
			result.Downloading++
		case seerrChangeSkipped:
			result.Skipped++
		}
	}
	return result, syncOutcome(result, firstErr)
}

// LibraryScanner asks for a scan of the libraries that can hold a title.
// A request completed from Seerr's "available" has a file somewhere the
// scanner has not necessarily seen yet, so the sync asks for a scan instead of
// waiting for the next scheduled one; the catalog's presence check then marks
// the request as in the library.
type LibraryScanner interface {
	ScanLibrariesFor(ctx context.Context, mediaType MediaType) error
}

// requestFulfillmentScan asks the wired scanner for a scan of the libraries
// that can hold the media type. A failure is logged and dropped: the completion
// already stands, and the next scheduled scan finds the file anyway.
func (s *Service) requestFulfillmentScan(ctx context.Context, mediaType MediaType) {
	if s == nil || s.libraryScanner == nil {
		return
	}
	if err := s.libraryScanner.ScanLibrariesFor(ctx, mediaType); err != nil {
		slog.WarnContext(ctx, "requests: scan after a Seerr completion failed", "component", "requests",
			"media_type", mediaType, "err", err)
	}
}

// seerrRetiredEvent is the history entry the retirement writes. A request whose
// newest entry is this one, and which is still active with nothing to check, is
// one whose closing write did not land.
const seerrRetiredEvent = "removed_in_seerr"

// closeUnfinishedRetirement closes a request that Seerr no longer has but that
// is still active, because the retirement that removed its targets did not
// finish closing it. Nothing about the state says so on its own — a request
// approved and waiting to be submitted looks the same — so the history decides:
// the retirement is the last thing that happened to it, so no later approval or
// retry stands behind it.
func (s *Service) closeUnfinishedRetirement(ctx context.Context, req Request) (seerrChange, error) {
	latest, err := s.store.LatestEventType(ctx, req.ID)
	if err != nil {
		return seerrChangeNone, err
	}
	if latest != seerrRetiredEvent {
		return seerrChangeSkipped, nil
	}
	if _, err := s.store.SetOutcome(ctx, req.ID, StateGuard{Outcomes: []Outcome{OutcomeActive}},
		OutcomeRemoved, Viewer{}, seerrRemovedMessage); err != nil {
		if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrNotFound) {
			return seerrChangeSkipped, nil // a person closed it meanwhile, or it is gone
		}
		return seerrChangeNone, err
	}
	return seerrChangeRemoved, nil
}

// syncOutcome turns a run's per-request failures into the run's own error. A run
// that got an answer for at least one request completed, with the failures
// counted in its result data and retried by the next run; a run Seerr answered
// nothing for reports the failure, so the task's history shows it and a first
// sweep stays owed.
func syncOutcome(result SeerrSyncResult, firstErr error) error {
	if firstErr == nil || result.Checked == 0 || result.Errors < result.Checked {
		return nil
	}
	return firstErr
}

// SyncSeerrRequestsAll reconciles every request that has a Seerr id, including
// Done and Failed ones, so a deployment's first run of this sync corrects
// entries left stale before it existed. It pages the admin list by creation
// cursor and applies the same mapping as SyncSeerrRequests. A request whose
// targets all completed is left alone: its Seerr request may be gone while the
// library still has the file, and removing the target would lose that record.
// Checked counts every request the pass walked, not only the ones it changed.
func (s *Service) SyncSeerrRequestsAll(ctx context.Context, limit int) (SeerrSyncResult, error) {
	result := SeerrSyncResult{}
	if s == nil || s.store == nil {
		return result, fmt.Errorf("request service is not configured")
	}
	reader := s.seerrStatusReader()
	if reader == nil {
		return result, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return result, err
	}
	if !hasSeerrConnection(fc) {
		return result, nil
	}
	var firstErr error
	var before *RequestPageKey
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		page, err := s.store.ListAdmin(ctx, ListFilter{Before: before, Limit: limit})
		if err != nil {
			return result, err
		}
		if len(page) == 0 {
			break
		}
		for _, req := range page {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			result.Checked++
			change, err := s.syncSeerrRequest(ctx, *req, fc, reader)
			if err != nil {
				result.Errors++
				slog.WarnContext(ctx, "seerr status sync failed", "component", "requests",
					"request_id", req.ID, "err", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			switch change {
			case seerrChangeDeclined:
				result.Declined++
			case seerrChangeRemoved:
				result.Removed++
			case seerrChangeCompleted:
				result.Completed++
			case seerrChangeDownloading:
				result.Downloading++
			case seerrChangeSkipped:
				result.Skipped++
			}
		}
		if len(page) < limit {
			break
		}
		last := page[len(page)-1]
		before = &RequestPageKey{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return result, syncOutcome(result, firstErr)
}

// SyncSeerrRequest reconciles one request with Seerr by id, for the admin's
// per-request refresh. It reads the request's Seerr status and applies the same
// mapping as the scheduled pass, then returns the request as stored. A server
// with no Seerr connection, or a Seerr that cannot be reached, reports
// ErrIntegrationUnreachable so the API answers an upstream-unavailable status
// rather than pretending the request is unchanged.
func (s *Service) SyncSeerrRequest(ctx context.Context, id string) (*Request, error) {
	if s == nil || s.store == nil {
		return nil, errRequestServiceNotConfigured
	}
	reader := s.seerrStatusReader()
	if reader == nil {
		return nil, ErrIntegrationUnreachable
	}
	req, err := s.store.GetRequest(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return nil, err
	}
	if !hasSeerrConnection(fc) {
		return nil, ErrIntegrationUnreachable
	}
	if _, err := s.syncSeerrRequest(ctx, *req, fc, reader); err != nil {
		return nil, err
	}
	return s.store.GetRequest(ctx, req.ID)
}

// seerrChange is what one request's sync did.
type seerrChange int

const (
	seerrChangeNone seerrChange = iota
	seerrChangeSkipped
	seerrChangeDeclined
	seerrChangeRemoved
	seerrChangeCompleted
	seerrChangeDownloading
)

// syncSeerrRequest reconciles one request's targets against Seerr. Targets that
// share a Seerr request are read once: Seerr keeps one request per title with
// its HD and 4K variants on one media row. A 404 retires those targets and,
// when none remain, closes the request as Removed. A declined request closes as
// Declined. A found request updates each target's status from its quality's
// media status. History records the change once per request.
//
// A request a person closed — declined, cancelled or removed — is left as it
// is. The full sweep walks those too, and Seerr's answer for one (usually a
// 404, since closing the request removed it there) must not rewrite what a
// person decided.
func (s *Service) syncSeerrRequest(ctx context.Context, req Request, fc *fulfillContext, reader SeerrStatusReader) (seerrChange, error) {
	switch req.Outcome {
	case OutcomeDeclined, OutcomeCancelled, OutcomeRemoved:
		return seerrChangeSkipped, nil
	}
	targets, err := s.store.ListTargets(ctx, req.ID)
	if err != nil {
		return seerrChangeNone, err
	}
	// Group targets by the Seerr request they belong to, so HD and 4K targets
	// sharing one request cost one lookup, not two.
	type group struct {
		in         Integration
		externalID string
		targets    []Target
	}
	groups := map[string]*group{}
	order := []string{}
	for _, target := range targets {
		if target.Status == StatusCompleted {
			continue
		}
		externalID := strings.TrimSpace(target.ExternalID)
		if externalID == "" {
			continue
		}
		in := integrationByID(fc, target.IntegrationID)
		if in == nil || !in.Enabled || in.CapabilityID != seerrCapabilityID {
			continue
		}
		key := in.ID + "\x00" + externalID
		if _, ok := groups[key]; !ok {
			groups[key] = &group{in: *in, externalID: externalID}
			order = append(order, key)
		}
		groups[key].targets = append(groups[key].targets, target)
	}
	if len(order) == 0 {
		// Nothing to read a status from. A request left here with no targets at
		// all is usually just approved and not yet submitted, which the
		// reconcile pass handles. One whose last history entry is the Seerr
		// retirement is a request Seerr no longer has whose closing write did
		// not land: left as it is, the reconcile pass would submit it again,
		// which is how a title Seerr dropped comes back. Close it now.
		if len(targets) == 0 {
			return s.closeUnfinishedRetirement(ctx, req)
		}
		return seerrChangeSkipped, nil
	}

	change := seerrChangeNone
	message := ""
	for _, key := range order {
		g := groups[key]
		status, err := reader.RequestStatus(ctx, g.in, g.externalID)
		if err != nil {
			// An unreachable or refusing Seerr must not read as "the request is
			// gone": the caller keeps the request and retries next run.
			return change, fmt.Errorf("%w: seerr: read request %s: %v", ErrIntegrationUnreachable, g.externalID, err)
		}
		if status == nil {
			// Seerr no longer has the request: retire the targets that came
			// from it. The last one removed leaves the request approved with
			// no targets (aggregateStatus treats an empty target list as
			// approved), so the helper closes it as Removed and records the
			// history line. Targets of another Seerr request or another
			// integration keep the request active, and the run counts it as
			// skipped rather than removed.
			_, removed, err := s.retireSeerrTargets(ctx, req, g.targets)
			if err != nil {
				return change, err
			}
			if removed {
				return seerrChangeRemoved, nil
			}
			if change == seerrChangeNone {
				change = seerrChangeSkipped
			}
			continue
		}
		if status.Declined {
			// Declined in Seerr: close the whole request, which records its
			// own history line. The guard leaves a request another actor
			// already closed alone.
			if _, err := s.store.SetOutcome(ctx, req.ID, StateGuard{Outcomes: []Outcome{OutcomeActive}},
				OutcomeDeclined, Viewer{}, seerrDeclinedMessage); err != nil {
				if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrNotFound) {
					return seerrChangeSkipped, nil
				}
				return change, err
			}
			return seerrChangeDeclined, nil
		}
		moved := make([]string, 0, len(g.targets))
		completed := false
		for _, target := range g.targets {
			next, changed := seerrTargetStatus(status, target)
			if !changed {
				continue
			}
			if _, err := s.store.UpdateTargetStatus(ctx, target.ID, next, "", "", "", Viewer{}); err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return change, err
			}
			moved = append(moved, fmt.Sprintf("%s %s", target.Quality, seerrStatusPhrase(next)))
			if next == StatusCompleted {
				change = seerrChangeCompleted
				completed = true
			} else if change == seerrChangeNone {
				change = seerrChangeDownloading
			}
		}
		if len(moved) > 0 {
			message = "Synced from Seerr: " + strings.Join(moved, ", ")
		}
		if completed {
			// Seerr has the media: the file is on disk, but the catalog may
			// not have scanned it yet. Ask for a scan of the libraries that
			// can hold it, so the request shows as in the library rather than
			// waiting for the next scheduled scan.
			s.requestFulfillmentScan(ctx, req.MediaType)
		}
	}
	if change == seerrChangeNone {
		return seerrChangeSkipped, nil
	}
	if message != "" {
		if err := s.store.RecordEvent(ctx, req.ID, "synced_from_seerr", Viewer{}, message); err != nil {
			return change, err
		}
	}
	return change, nil
}
