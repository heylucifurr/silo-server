package requests

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// TargetService is the host's download-service client (Seerr). DeleteTarget
// removes a fulfilled target; TargetExists reports whether the target's
// request still exists there, so a request deleted in Seerr directly is
// retired in Silo instead of sitting in flight forever. Both return nil/true
// for a connection of another kind, which the host leaves alone.
type TargetService interface {
	DeleteTarget(ctx context.Context, in Integration, target Target, opts DeleteOptions) error
	TargetExists(ctx context.Context, in Integration, target Target) (bool, error)
}

// SetTargetService wires the download-service client DeleteRequest and the
// reconcile pass use.
func (s *Service) SetTargetService(t TargetService) { s.targets = t }

// DeleteRequest removes a request from Silo, admin only. For each selected
// quality the target is removed from its download service first (unless
// SiloOnly, or it was never submitted), then from Silo. When no target
// remains the request is soft-removed: the row and its history stay, so
// reconciliation and the audit trail keep working, but it stops counting as
// active and leaves the queue. A downstream failure stops the delete with the
// Silo record untouched, so the admin can retry or choose Silo-only. Deleting
// an already-removed request is a no-op.
func (s *Service) DeleteRequest(ctx context.Context, viewer Viewer, id string, opts DeleteOptions) (*Request, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	if s == nil || s.store == nil {
		return nil, errors.New("requests: service is not configured")
	}
	req, err := s.store.GetRequest(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if req.Outcome == OutcomeRemoved {
		return req, nil
	}
	targets, err := s.store.ListTargets(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	selected := selectDeleteTargets(targets, opts.Qualities)
	if len(selected) == 0 {
		// Nothing was ever sent for the selected qualities: close the request.
		return s.removeRequest(ctx, req, viewer, opts)
	}
	var updated *Request
	for _, target := range selected {
		if err := s.deleteDownstream(ctx, target, opts); err != nil {
			return nil, err
		}
		req2, err := s.store.RemoveTarget(ctx, target.ID, viewer)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // another actor removed it first
			}
			return nil, err
		}
		updated = req2
		if err := s.store.RecordEvent(ctx, req.ID, "target_deleted", viewer, deleteMessage(target, opts)); err != nil {
			return nil, err
		}
	}
	remaining, err := s.store.ListTargets(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if len(remaining) == 0 {
		return s.removeRequest(ctx, req, viewer, opts)
	}
	if updated != nil {
		return updated, nil
	}
	return s.store.GetRequest(ctx, req.ID)
}

// deleteDownstream hands a submitted target to the download-service deleter.
// A target with no external id was never sent, and one whose server is gone or
// disabled has nothing to remove, so both are removed from Silo only.
func (s *Service) deleteDownstream(ctx context.Context, target Target, opts DeleteOptions) error {
	if opts.SiloOnly || s.targets == nil {
		return nil
	}
	if strings.TrimSpace(target.ExternalID) == "" || strings.TrimSpace(target.IntegrationID) == "" {
		return nil
	}
	in, err := s.store.GetIntegration(ctx, target.IntegrationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if in == nil || !in.Enabled {
		return nil
	}
	return s.targets.DeleteTarget(ctx, *in, target, opts)
}

// removeRequest soft-removes a request: the row and its history stay, the
// outcome becomes removed, and the title's followers are dropped.
func (s *Service) removeRequest(ctx context.Context, req *Request, viewer Viewer, opts DeleteOptions) (*Request, error) {
	return s.store.SetOutcome(ctx, req.ID, StateGuard{}, OutcomeRemoved, viewer, deleteMessage(Target{Quality: ""}, opts))
}

// selectDeleteTargets narrows a request's targets to the chosen qualities;
// an empty set selects every target.
func selectDeleteTargets(targets []Target, qualities []Quality) []Target {
	if len(qualities) == 0 {
		return targets
	}
	var out []Target
	for _, t := range targets {
		if slices.Contains(qualities, t.Quality) {
			out = append(out, t)
		}
	}
	return out
}

// deleteMessage records what an admin's delete did, for the request history.
func deleteMessage(target Target, opts DeleteOptions) string {
	var b strings.Builder
	b.WriteString("deleted")
	if target.Quality != "" {
		b.WriteString(" " + string(target.Quality))
	}
	switch {
	case opts.SiloOnly:
		b.WriteString(" from Silo only")
	case opts.DeleteFiles:
		b.WriteString(", its download and its files")
	case opts.RemoveMedia:
		b.WriteString(" and its download")
	}
	return b.String()
}
