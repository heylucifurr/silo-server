package requests

import (
	"context"
	"strings"
)

// seerrRemovedMessage is the history line for a request Seerr no longer has.
// The reconcile pass and the status sync retire targets with it, so both paths
// read the same in a request's history.
const seerrRemovedMessage = "Synced from Seerr: the request no longer exists in Seerr"

// retireMissingSeerrTargets removes the request's targets whose Seerr request
// no longer exists, so a request deleted in Seerr directly is retired in Silo
// instead of sitting in Processing forever. A completed target is left alone:
// an imported copy is not a deletion. It returns the request after the
// removals (nil when nothing changed) and whether the request ended up
// removed. A Seerr lookup that fails is returned as an error, and the caller
// keeps the request as it is rather than deleting it on a guess.
func (s *Service) retireMissingSeerrTargets(ctx context.Context, req Request, targets []Target, fc *fulfillContext) (*Request, bool, error) {
	if s.targets == nil {
		return nil, false, nil
	}
	var missing []Target
	for _, target := range targets {
		if target.Status == StatusCompleted {
			continue
		}
		if strings.TrimSpace(target.ExternalID) == "" || strings.TrimSpace(target.IntegrationID) == "" {
			continue
		}
		in := integrationByID(fc, target.IntegrationID)
		if in == nil || !in.Enabled || in.CapabilityID != seerrCapabilityID {
			continue
		}
		exists, err := s.targets.TargetExists(ctx, *in, target)
		if err != nil {
			return nil, false, err
		}
		if exists {
			continue
		}
		missing = append(missing, target)
	}
	if len(missing) == 0 {
		return nil, false, nil
	}
	return s.retireSeerrTargets(ctx, req, missing)
}

// retireSeerrTargets retires targets Seerr no longer has and, when none are
// left, closes the request as Removed with its row and history kept. The
// deletions and the closing write are one call: a request left approved and
// active with no targets is one the reconcile pass would submit, which is how a
// title Seerr dropped comes back. It reports whether the request ended up
// removed.
//
// The status sync calls it once Seerr answers 404 for a request, the reconcile
// pass once its own existence check says the same.
func (s *Service) retireSeerrTargets(ctx context.Context, req Request, targets []Target) (*Request, bool, error) {
	ids := make([]int64, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	updated, removed, err := s.store.RemoveTargetsAndClose(ctx, req.ID, ids,
		StateGuard{Outcomes: []Outcome{OutcomeActive}}, Viewer{}, seerrRemovedMessage)
	if err != nil || !removed {
		return updated, removed, err
	}
	// The closing write records the outcome; this names where it came from, and
	// is the marker the next run looks for on a request whose close did not
	// land.
	if err := s.store.RecordEvent(ctx, req.ID, seerrRetiredEvent, Viewer{}, seerrRemovedMessage); err != nil {
		return updated, removed, err
	}
	return updated, removed, nil
}
