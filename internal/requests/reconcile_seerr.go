package requests

import (
	"context"
	"errors"
	"strings"
)

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
	var updated *Request
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
			return updated, false, err
		}
		if exists {
			continue
		}
		next, err := s.store.RemoveTarget(ctx, target.ID, Viewer{})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // another actor removed it first
			}
			return updated, false, err
		}
		updated = next
		if err := s.store.RecordEvent(ctx, req.ID, "removed_in_seerr", Viewer{}, "removed: the request no longer exists in Seerr"); err != nil {
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
	final, err := s.store.SetOutcome(ctx, req.ID, StateGuard{}, OutcomeRemoved, Viewer{}, "removed: the request no longer exists in Seerr")
	if err != nil {
		return updated, false, err
	}
	return final, true, nil
}
