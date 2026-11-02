package heroclips

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAnticipatedCandidateIDRoundTrip(t *testing.T) {
	id := AnticipatedCandidateID("movie", 1083381)
	if id != "ext:movie-1083381" {
		t.Fatalf("AnticipatedCandidateID = %q", id)
	}
	mediaType, tmdbID, ok := parseAnticipatedCandidateID(id)
	if !ok || mediaType != "movie" || tmdbID != 1083381 {
		t.Fatalf("parse(%q) = %q, %d, %v", id, mediaType, tmdbID, ok)
	}

	// A catalog id is not an anticipated id, and neither is a malformed one.
	for _, bad := range []string{"tvdb-372264", "ext:", "ext:book-1", "ext:movie-", "ext:movie-0", "ext:movie-x"} {
		if _, _, ok := parseAnticipatedCandidateID(bad); ok {
			t.Fatalf("parse(%q) reported a valid id", bad)
		}
	}
}

// anticipatedClipPool connects to SILO_TEST_DATABASE_URL and clears the curated
// tables, skipping when it is unset.
func anticipatedClipPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	clear := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM public.anticipated_clips`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM public.anticipated_titles`)
	}
	clear()
	t.Cleanup(clear)
	return pool
}

func TestAnticipatedClipCandidatesAndPathsDB(t *testing.T) {
	ctx := t.Context()
	pool := anticipatedClipPool(t)

	// Two curated titles with a trailer: one gets a clip at the current recipe.
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.anticipated_titles (id, media_type, tmdb_id, position, title, trailer_site, trailer_key)
		VALUES (1, 'movie', 101, 0, 'Has clip', 'youtube', 'aaa'),
		       (2, 'series', 202, 1, 'Needs clip', 'youtube', 'bbb')`); err != nil {
		t.Fatalf("seed titles: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.anticipated_clips (media_type, tmdb_id, clip_path, recipe, state)
		VALUES ('movie', 101, 'hero-clips/aaaa/v1-w1280-crf28-a128k.mp4', 'v1-w1280-crf28-a128k', 'ready')`); err != nil {
		t.Fatalf("seed clip: %v", err)
	}

	repo := NewRepository(pool)
	candidates, err := repo.AnticipatedCandidates(ctx, "v1-w1280-crf28-a128k", 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("AnticipatedCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ContentID != AnticipatedCandidateID("series", 202) ||
		candidates[0].SiteKey != "bbb" {
		t.Fatalf("candidates = %+v, want only the series without a clip", candidates)
	}

	// Every seeded title gets its clip made for a newer recipe.
	newer, err := repo.AnticipatedCandidates(ctx, "v2-w1920-crf28-a128k", 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("AnticipatedCandidates (newer recipe): %v", err)
	}
	if len(newer) != 2 {
		t.Fatalf("a recipe change must rebuild every clip, got %d", len(newer))
	}

	// Only the ready clip resolves, keyed by the synthetic id the card uses.
	paths, err := repo.AnticipatedClipPathsFor(ctx, []string{
		AnticipatedCandidateID("movie", 101),
		AnticipatedCandidateID("series", 202),
		"tvdb-372264",
	})
	if err != nil {
		t.Fatalf("AnticipatedClipPathsFor: %v", err)
	}
	if len(paths) != 1 || paths[AnticipatedCandidateID("movie", 101)] == "" {
		t.Fatalf("paths = %+v, want only the movie's ready clip", paths)
	}

	// A failed clip is not offered to a client.
	if _, err := pool.Exec(ctx, `
		UPDATE public.anticipated_clips SET state = 'failed' WHERE media_type = 'movie'`); err != nil {
		t.Fatalf("fail clip: %v", err)
	}
	paths, err = repo.AnticipatedClipPathsFor(ctx, []string{AnticipatedCandidateID("movie", 101)})
	if err != nil {
		t.Fatalf("AnticipatedClipPathsFor after failure: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("a failed clip must not resolve: %+v", paths)
	}

	// And a failed retry never clears a clip that is already ready.
	publisher := NewAnticipatedPublisher(pool)
	if err := publisher.PublishReady(ctx, Clip{
		ContentID: AnticipatedCandidateID("movie", 101), Path: "hero-clips/aaaa/v1-w1280-crf28-a128k.mp4",
		Recipe: "v1-w1280-crf28-a128k", Bytes: 1, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("PublishReady: %v", err)
	}
	if err := publisher.RecordFailure(ctx, AnticipatedCandidateID("movie", 101), "v1-w1280-crf28-a128k", "youtube", "aaa", "boom"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM public.anticipated_clips WHERE media_type='movie'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "ready" {
		t.Fatalf("a failed retry cleared a ready clip: state = %q", state)
	}
}
