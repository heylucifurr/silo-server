package anticipated

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/idgen"
)

// Store holds the SQL for anticipated_titles and anticipated_aliases. There is no
// content_id column: entries are matched to the library by provider ID at read
// time, never by a stored catalog id.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const titleColumns = `id, media_type, tmdb_id, imdb_id, coalesce(tvdb_id, 0), position, title, original_title,
	coalesce(year, 0), release_date, overview, poster_path, backdrop_path, logo_path,
	coalesce(vote_average, 0)::float8, coalesce(runtime, 0), coalesce(episode_runtime, 0), certification,
	trailer_site, trailer_key, state, last_checked_at`

func scanTitle(row pgx.Row) (Title, error) {
	var t Title
	err := row.Scan(
		&t.ID, &t.MediaType, &t.TMDBID, &t.IMDbID, &t.TVDBID, &t.Position, &t.Title, &t.OriginalTitle,
		&t.Year, &t.ReleaseDate, &t.Overview, &t.PosterPath, &t.BackdropPath, &t.LogoPath,
		&t.VoteAverage, &t.Runtime, &t.EpisodeRuntime, &t.Certification,
		&t.TrailerSite, &t.TrailerKey, &t.State, &t.LastCheckedAt,
	)
	return t, err
}

// List returns every entry in position order.
func (s *Store) List(ctx context.Context) ([]Title, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+titleColumns+` FROM anticipated_titles ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("listing anticipated titles: %w", err)
	}
	defer rows.Close()
	out := make([]Title, 0, 16)
	for rows.Next() {
		t, err := scanTitle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReplaceAll replaces the whole list in one transaction. An entry already present
// (matched on media_type + tmdb_id) keeps its snapshot; a new one is inserted bare
// and resolved by the service. Positions come from the input order. Aliases are
// (re)recorded for every title's current provider IDs.
func (s *Store) ReplaceAll(ctx context.Context, in []Input) ([]Title, error) {
	if err := ValidateInputs(in); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning anticipated replace: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mediaTypes := make([]string, len(in))
	tmdbIDs := make([]int, len(in))
	for i, e := range in {
		mediaTypes[i] = e.MediaType
		tmdbIDs[i] = e.TMDBID
	}

	// Drop entries the admin removed.
	if _, err := tx.Exec(ctx, `
		DELETE FROM anticipated_titles t
		WHERE NOT EXISTS (
			SELECT 1 FROM unnest($1::text[], $2::int[]) AS keep(media_type, tmdb_id)
			WHERE keep.media_type = t.media_type AND keep.tmdb_id = t.tmdb_id
		)`, mediaTypes, tmdbIDs); err != nil {
		return nil, fmt.Errorf("pruning anticipated titles: %w", err)
	}

	// Upsert in input order. A new entry gets a minted id; an existing one keeps
	// its snapshot columns.
	for i, e := range in {
		id, err := mintID()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO anticipated_titles (id, media_type, tmdb_id, position)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (media_type, tmdb_id)
			DO UPDATE SET position = EXCLUDED.position, updated_at = now()`,
			id, e.MediaType, e.TMDBID, i); err != nil {
			return nil, fmt.Errorf("upserting anticipated title %s %d: %w", e.MediaType, e.TMDBID, err)
		}
	}

	// Record each title's current provider IDs as aliases.
	if _, err := tx.Exec(ctx, `
		INSERT INTO anticipated_aliases (title_id, media_type, provider, provider_id)
		SELECT t.id, t.media_type, 'tmdb', t.tmdb_id::text FROM anticipated_titles t
		UNION ALL
		SELECT t.id, t.media_type, 'imdb', t.imdb_id FROM anticipated_titles t WHERE t.imdb_id <> ''
		UNION ALL
		SELECT t.id, t.media_type, 'tvdb', t.tvdb_id::text FROM anticipated_titles t WHERE t.tvdb_id IS NOT NULL
		ON CONFLICT DO NOTHING`); err != nil {
		return nil, fmt.Errorf("recording anticipated aliases: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing anticipated replace: %w", err)
	}
	return s.List(ctx)
}

// mintID returns a new Sonyflake id as an int64.
func mintID() (int64, error) {
	s, err := idgen.NextID()
	if err != nil {
		return 0, fmt.Errorf("minting anticipated title id: %w", err)
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing anticipated title id %q: %w", s, err)
	}
	return id, nil
}

// UpsertSnapshot writes a refreshed entry's snapshot fields, provider IDs and
// trailer by id, and clears its failure bookkeeping.
func (s *Store) UpsertSnapshot(ctx context.Context, t Title) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE anticipated_titles SET
			imdb_id = $2, tvdb_id = $3, title = $4, original_title = $5, year = $6,
			release_date = $7, overview = $8, poster_path = $9, backdrop_path = $10, logo_path = $11,
			vote_average = $12, runtime = $13, episode_runtime = $14, certification = $15,
			trailer_site = $16, trailer_key = $17, state = 'active', not_found_count = 0,
			last_checked_at = now(), next_check_at = now() + interval '24 hours', updated_at = now()
		WHERE id = $1`,
		t.ID, t.IMDbID, nullableInt(t.TVDBID), t.Title, t.OriginalTitle, nullableInt(t.Year),
		t.ReleaseDate, t.Overview, t.PosterPath, t.BackdropPath, t.LogoPath,
		t.VoteAverage, t.Runtime, t.EpisodeRuntime, t.Certification,
		t.TrailerSite, t.TrailerKey)
	if err != nil {
		return fmt.Errorf("updating anticipated snapshot: %w", err)
	}
	return nil
}

// DueForRefresh returns active entries whose next check is due, oldest first.
func (s *Store) DueForRefresh(ctx context.Context, now time.Time, limit int) ([]Title, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+titleColumns+` FROM anticipated_titles
		WHERE state = 'active' AND (next_check_at IS NULL OR next_check_at <= $1)
		ORDER BY next_check_at NULLS FIRST, id
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("listing anticipated titles due for refresh: %w", err)
	}
	defer rows.Close()
	out := make([]Title, 0, limit)
	for rows.Next() {
		t, err := scanTitle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkNotFound records a failed read: needs_review once a second read misses, and
// removed once the count reaches threshold. The entry is never deleted.
func (s *Store) MarkNotFound(ctx context.Context, id int64, threshold int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE anticipated_titles SET
			not_found_count = not_found_count + 1,
			last_not_found_at = now(),
			last_checked_at = now(),
			next_check_at = now() + interval '24 hours',
			state = CASE
				WHEN not_found_count + 1 >= $2 THEN 'removed'
				WHEN not_found_count + 1 >= 2 THEN 'needs_review'
				ELSE state END,
			updated_at = now()
		WHERE id = $1`, id, threshold)
	if err != nil {
		return fmt.Errorf("recording anticipated not-found: %w", err)
	}
	return nil
}

// nullableInt maps 0 to SQL NULL, for the nullable integer columns.
func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}
