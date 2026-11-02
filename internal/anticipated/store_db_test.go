package anticipated

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore connects to SILO_TEST_DATABASE_URL and clears the curated tables.
// The list is server-wide, so the tables are shared with nothing else and can be
// truncated per test.
func testStore(t *testing.T) *Store {
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM public.anticipated_titles`)
	}
	clear()
	t.Cleanup(clear)
	return NewStore(pool)
}

func TestAnticipatedStoreReplaceAndList(t *testing.T) {
	ctx := t.Context()
	store := testStore(t)

	movie := Input{MediaType: MediaMovie, TMDBID: 101}
	series := Input{MediaType: MediaSeries, TMDBID: 202}

	list, err := store.ReplaceAll(ctx, []Input{movie, series})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].TMDBID != 101 || list[0].Position != 0 || list[1].TMDBID != 202 || list[1].Position != 1 {
		t.Fatalf("list order = %+v", list)
	}
	seriesID := list[1].ID

	// A refresh writes the second entry's snapshot.
	release := time.Date(2026, 12, 18, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertSnapshot(ctx, Title{
		ID: seriesID, Title: "Kept Title", OriginalTitle: "Kept Title", Year: 2026,
		ReleaseDate: &release, TrailerSite: "youtube", TrailerKey: "abc123",
	}); err != nil {
		t.Fatalf("UpsertSnapshot: %v", err)
	}

	// Reordering keeps both entries and reassigns positions.
	list, err = store.ReplaceAll(ctx, []Input{series, movie})
	if err != nil {
		t.Fatalf("ReplaceAll reorder: %v", err)
	}
	if list[0].TMDBID != 202 || list[0].Position != 0 || list[1].TMDBID != 101 || list[1].Position != 1 {
		t.Fatalf("reordered list = %+v", list)
	}

	// Dropping the first keeps the second's row (same id) and its snapshot.
	list, err = store.ReplaceAll(ctx, []Input{series})
	if err != nil {
		t.Fatalf("ReplaceAll drop: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	got := list[0]
	if got.ID != seriesID {
		t.Fatalf("kept a new row: id = %d, want %d", got.ID, seriesID)
	}
	if got.Title != "Kept Title" || got.TrailerKey != "abc123" {
		t.Fatalf("snapshot lost: %+v", got)
	}
	if got.Position != 0 {
		t.Fatalf("position = %d, want 0", got.Position)
	}

	// Re-adding the same pair does not create a second row.
	list, err = store.ReplaceAll(ctx, []Input{series, movie, series})
	if err != nil {
		t.Fatalf("ReplaceAll re-add: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("duplicate pair created extra rows: len = %d, want 2", len(list))
	}
}
