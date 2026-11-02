package heroclips

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AnticipatedClipPrefix marks a clip candidate whose title the library does not
// own. It is deliberately the same string the sections package uses for an
// unowned anticipated slide item (sections.ExternalContentIDPrefix), so a clip
// and the card that plays it share one identity; a test in internal/api/handlers
// pins the two together. The rest is "<media_type>-<tmdb_id>".
const AnticipatedClipPrefix = "ext:"

// AnticipatedCandidateID builds the synthetic id for an unowned curated title.
func AnticipatedCandidateID(mediaType string, tmdbID int) string {
	return AnticipatedClipPrefix + mediaType + "-" + strconv.Itoa(tmdbID)
}

// parseAnticipatedCandidateID splits a synthetic id back into its parts,
// reporting false for anything that is not one.
func parseAnticipatedCandidateID(contentID string) (mediaType string, tmdbID int, ok bool) {
	rest, found := strings.CutPrefix(contentID, AnticipatedClipPrefix)
	if !found {
		return "", 0, false
	}
	mediaType, id, found := strings.Cut(rest, "-")
	if !found || (mediaType != "movie" && mediaType != "series") {
		return "", 0, false
	}
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return mediaType, n, true
}

// AnticipatedPublisher records what a generation produced for an unowned curated
// title, in anticipated_clips. It is the twin of the Repository's hero_clips
// writes: the same publish-last order, and the same rule that a failed retry
// never clears a clip that is already ready.
type AnticipatedPublisher struct{ pool *pgxpool.Pool }

// NewAnticipatedPublisher creates a publisher backed by pool.
func NewAnticipatedPublisher(pool *pgxpool.Pool) *AnticipatedPublisher {
	return &AnticipatedPublisher{pool: pool}
}

func (p *AnticipatedPublisher) PublishReady(ctx context.Context, clip Clip) error {
	mediaType, tmdbID, ok := parseAnticipatedCandidateID(clip.ContentID)
	if !ok {
		return fmt.Errorf("anticipated clip: %q is not an anticipated id", clip.ContentID)
	}
	if p == nil || p.pool == nil {
		return fmt.Errorf("anticipated clips: no database")
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO public.anticipated_clips (
			media_type, tmdb_id, clip_path, recipe, source, source_key,
			duration_ms, width, height, bytes, state, error, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'ready', '', now())
		ON CONFLICT (media_type, tmdb_id) DO UPDATE SET
			clip_path = EXCLUDED.clip_path,
			recipe = EXCLUDED.recipe,
			source = EXCLUDED.source,
			source_key = EXCLUDED.source_key,
			duration_ms = EXCLUDED.duration_ms,
			width = EXCLUDED.width,
			height = EXCLUDED.height,
			bytes = EXCLUDED.bytes,
			state = 'ready',
			error = '',
			updated_at = now()`,
		mediaType, tmdbID, clip.Path, clip.Recipe, clip.Source, clip.SourceKey,
		clip.DurationMS, clip.Width, clip.Height, clip.Bytes)
	if err != nil {
		return fmt.Errorf("publish anticipated clip: %w", err)
	}
	return nil
}

func (p *AnticipatedPublisher) RecordFailure(ctx context.Context, contentID, recipe, source, sourceKey, message string) error {
	mediaType, tmdbID, ok := parseAnticipatedCandidateID(contentID)
	if !ok {
		return fmt.Errorf("anticipated clip: %q is not an anticipated id", contentID)
	}
	if p == nil || p.pool == nil {
		return fmt.Errorf("anticipated clips: no database")
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO public.anticipated_clips (media_type, tmdb_id, recipe, source, source_key, state, error, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'failed', $6, now())
		ON CONFLICT (media_type, tmdb_id) DO UPDATE SET
			recipe = EXCLUDED.recipe,
			source = EXCLUDED.source,
			source_key = EXCLUDED.source_key,
			state = CASE WHEN public.anticipated_clips.state = 'ready' THEN 'ready' ELSE 'failed' END,
			error = EXCLUDED.error,
			updated_at = now()`,
		mediaType, tmdbID, recipe, source, sourceKey, truncateError(message))
	if err != nil {
		return fmt.Errorf("record anticipated clip failure: %w", err)
	}
	return nil
}

// entries that have a trailer and are missing a clip, holding one an older
// recipe made, or failed long enough ago to retry. Entries removed from the list
// or marked removed are left out, so their clips stop being regenerated.
func (r *Repository) AnticipatedCandidates(ctx context.Context, recipe string, retryAfter time.Duration, limit int) ([]Candidate, error) {
	if r == nil || r.pool == nil {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT '`+AnticipatedClipPrefix+`' || t.media_type || '-' || t.tmdb_id::text,
		       t.trailer_site, t.trailer_key
		FROM public.anticipated_titles t
		LEFT JOIN public.anticipated_clips ac
		  ON ac.media_type = t.media_type AND ac.tmdb_id = t.tmdb_id
		WHERE t.state = 'active'
		  AND t.trailer_site <> '' AND t.trailer_key <> ''
		  AND (
			ac.media_type IS NULL
			OR ac.recipe <> $1
			OR (ac.state = 'failed' AND ac.updated_at < now() - $2::interval)
		  )
		ORDER BY t.position, t.id
		LIMIT $3`, recipe, retryAfter.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("load anticipated clip candidates: %w", err)
	}
	defer rows.Close()

	out := make([]Candidate, 0, limit)
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ContentID, &c.Site, &c.SiteKey); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AnticipatedClipPathsFor returns the stored path of the ready clip each of these
// synthetic ids has, keyed by that id. A row that records a failure is left out,
// so a client is never handed a URL for a clip that does not exist.
func (r *Repository) AnticipatedClipPathsFor(ctx context.Context, contentIDs []string) (map[string]string, error) {
	if r == nil || r.pool == nil || len(contentIDs) == 0 {
		return nil, nil
	}

	type key struct {
		mediaType string
		tmdbID    int
	}
	keys := make([]key, 0, len(contentIDs))
	mediaTypes := make([]string, 0, len(contentIDs))
	tmdbIDs := make([]int, 0, len(contentIDs))
	for _, contentID := range contentIDs {
		mediaType, tmdbID, ok := parseAnticipatedCandidateID(contentID)
		if !ok {
			continue
		}
		keys = append(keys, key{mediaType: mediaType, tmdbID: tmdbID})
		mediaTypes = append(mediaTypes, mediaType)
		tmdbIDs = append(tmdbIDs, tmdbID)
	}
	if len(keys) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT media_type, tmdb_id, clip_path
		FROM public.anticipated_clips ac
		WHERE state = 'ready' AND clip_path <> ''
		  AND (media_type, tmdb_id) IN (
			SELECT * FROM unnest($1::text[], $2::int[])
		  )`, mediaTypes, tmdbIDs)
	if err != nil {
		return nil, fmt.Errorf("load anticipated clips: %w", err)
	}
	defer rows.Close()

	byKey := make(map[key]string, len(keys))
	for rows.Next() {
		var k key
		var path string
		if err := rows.Scan(&k.mediaType, &k.tmdbID, &path); err != nil {
			return nil, err
		}
		byKey[k] = path
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	paths := make(map[string]string, len(byKey))
	for _, contentID := range contentIDs {
		mediaType, tmdbID, ok := parseAnticipatedCandidateID(contentID)
		if !ok {
			continue
		}
		if path, found := byKey[key{mediaType: mediaType, tmdbID: tmdbID}]; found {
			paths[contentID] = path
		}
	}
	return paths, nil
}
