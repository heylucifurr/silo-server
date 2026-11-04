package requests

import (
	"context"
	"errors"
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

// retireSeerrTargets removes targets Seerr no longer has and, when none are
// left, closes the request as Removed with its row and history kept. It
// returns the request after the removals (nil when none of the targets was
// still there) and whether the request ended up removed.
//
// The status sync calls it once Seerr answers 404 for a request, the reconcile
// pass once its own existence check says the same. Closing is guarded on an
// active request: one another actor closed meanwhile, or a target that was
// already gone, leaves the request as it is.
func (s *Service) retireSeerrTargets(ctx context.Context, req Request, targets []Target) (*Request, bool, error) {
	var updated *Request
	for _, target := range targets {
		next, err := s.store.RemoveTarget(ctx, target.ID, Viewer{})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // another actor removed it first
			}
			return updated, false, err
		}
		updated = next
		if err := s.store.RecordEvent(ctx, req.ID, "removed_in_seerr", Viewer{}, seerrRemovedMessage); err != nil {
			return updated, false, err
		}
	}
	if updated == nil {
		return nil, false, nil
	}
	remaining, err := s.store.ListTargets(ctx, req.ID)
	if err != nil {
		return updated, false, err
	}
	if len(remaining) > 0 {
		return updated, false, nil
	}
	final, err := s.store.SetOutcome(ctx, req.ID, StateGuard{Outcomes: []Outcome{OutcomeActive}},
		OutcomeRemoved, Viewer{}, seerrRemovedMessage)
	if err != nil {
		if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrNotFound) {
			// A person closed the request, or it is gone: the targets are
			// retired either way, and the outcome is theirs to keep.
			return updated, false, nil
		}
		return updated, false, err
	}
	return final, true, nil
}
