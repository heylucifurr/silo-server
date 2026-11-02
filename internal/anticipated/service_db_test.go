package anticipated

import (
	"testing"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

func TestServiceReplaceResolvesNewEntries(t *testing.T) {
	ctx := t.Context()
	store := testStore(t)
	client := &fakeTMDB{
		detail: &tmdb.MediaDetail{
			Title: "Doomsday", ReleaseDate: "2026-12-18", Overview: "…",
			PosterPath: "/p.jpg", BackdropPath: "/b.jpg", VoteAverage: 8.2,
		},
		videos: []tmdb.Video{{Site: "youtube", Key: "t1", Type: "Trailer", Official: true}},
	}
	svc := NewService(store, client)

	list, err := svc.Replace(ctx, []Input{{MediaType: MediaMovie, TMDBID: 550}})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if len(list) != 1 || list[0].Title != "Doomsday" || list[0].TrailerKey != "t1" {
		t.Fatalf("list = %+v", list)
	}

	// Replacing the same pair does not re-resolve: the snapshot already exists.
	before := client.detailCalls
	if _, err := svc.Replace(ctx, []Input{{MediaType: MediaMovie, TMDBID: 550}}); err != nil {
		t.Fatalf("Replace again: %v", err)
	}
	if client.detailCalls != before {
		t.Fatalf("detail calls = %d, want %d (no re-resolve)", client.detailCalls, before)
	}
}

func TestServiceRefreshKeepsLastGoodSnapshotOnError(t *testing.T) {
	ctx := t.Context()
	store, pool := testStoreWithPool(t)
	good := &fakeTMDB{
		detail: &tmdb.MediaDetail{Title: "Kept", ReleaseDate: "2026-05-05"},
	}
	svc := NewService(store, good)
	if _, err := svc.Replace(ctx, []Input{{MediaType: MediaMovie, TMDBID: 7}}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// A successful resolve schedules the next check a day out; bring it forward.
	if _, err := pool.Exec(ctx, `UPDATE anticipated_titles SET next_check_at = now() - interval '1 hour'`); err != nil {
		t.Fatalf("make the entry due: %v", err)
	}

	// The next read fails: the snapshot must survive and the failure recorded.
	svc.tmdb = &fakeTMDB{}
	if _, err := svc.Refresh(ctx, 10); err == nil {
		t.Fatal("Refresh: want an error for the failed entry")
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Title != "Kept" {
		t.Fatalf("snapshot lost: %+v", list)
	}
}
