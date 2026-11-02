package anticipated

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// notFoundThreshold is how many consecutive failed TMDB reads mark an entry
// removed. Two reads missing already move it to needs_review.
const notFoundThreshold = 3

// Service is the curation surface behind the admin API and the refresh task.
type Service struct {
	store *Store
	tmdb  TMDB
}

// NewService creates a Service over store, reading TMDB through client.
func NewService(store *Store, client TMDB) *Service {
	return &Service{store: store, tmdb: client}
}

// List returns the curated list in position order.
func (s *Service) List(ctx context.Context) ([]Title, error) {
	return s.store.List(ctx)
}

// Replace writes the whole list: it resolves any entry that has no snapshot yet
// (a newly added title) through TMDB, then stores the list and returns it.
func (s *Service) Replace(ctx context.Context, in []Input) ([]Title, error) {
	if err := ValidateInputs(in); err != nil {
		return nil, err
	}
	stored, err := s.store.ReplaceAll(ctx, in)
	if err != nil {
		return nil, err
	}
	for _, t := range stored {
		if strings.TrimSpace(t.Title) != "" {
			continue // already has a snapshot
		}
		resolved, err := resolveSnapshot(ctx, s.tmdb, Input{MediaType: t.MediaType, TMDBID: t.TMDBID})
		if err != nil {
			return nil, err
		}
		resolved.ID = t.ID
		if err := s.store.UpsertSnapshot(ctx, resolved); err != nil {
			return nil, err
		}
	}
	return s.store.List(ctx)
}

// Search returns TMDB titles for the curation picker.
func (s *Service) Search(ctx context.Context, mediaType, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Result{}, nil
	}
	page, err := s.tmdb.SearchMedia(ctx, tmdbMediaType(mediaType), query, 1)
	if err != nil {
		return nil, fmt.Errorf("searching TMDB: %w", err)
	}
	out := make([]Result, 0, len(page.Results))
	for _, r := range page.Results {
		mt := MediaMovie
		if r.MediaType == MediaSeries {
			mt = MediaSeries
		}
		out = append(out, Result{
			MediaType:    mt,
			TMDBID:       r.ID,
			Title:        r.Title,
			Year:         r.Year,
			ReleaseDate:  r.ReleaseDate,
			PosterPath:   r.PosterPath,
			BackdropPath: r.BackdropPath,
			Overview:     r.Overview,
			VoteAverage:  r.VoteAverage,
		})
	}
	return out, nil
}

// Refresh re-reads the entries due for a check. A failed read keeps the last good
// snapshot and records the failure; the batch continues, and the first failure is
// returned once every entry has been tried.
func (s *Service) Refresh(ctx context.Context, limit int) (int, error) {
	due, err := s.store.DueForRefresh(ctx, time.Now(), limit)
	if err != nil {
		return 0, err
	}
	refreshed := 0
	var firstErr error
	for _, t := range due {
		resolved, err := resolveSnapshot(ctx, s.tmdb, Input{MediaType: t.MediaType, TMDBID: t.TMDBID})
		if err != nil {
			if markErr := s.store.MarkNotFound(ctx, t.ID, notFoundThreshold); markErr != nil && firstErr == nil {
				firstErr = markErr
			} else if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resolved.ID = t.ID
		if err := s.store.UpsertSnapshot(ctx, resolved); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		refreshed++
	}
	if refreshed == 0 && len(due) > 0 && firstErr == nil {
		firstErr = errors.New("anticipated: no entries refreshed")
	}
	return refreshed, firstErr
}
