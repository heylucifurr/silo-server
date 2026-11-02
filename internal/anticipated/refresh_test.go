package anticipated

import (
	"context"
	"errors"
	"testing"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

type fakeTMDB struct {
	detail      *tmdb.MediaDetail
	detailErr   error
	videos      []tmdb.Video
	videosErr   error
	detailCalls int
}

func (f *fakeTMDB) GetMediaDetail(context.Context, string, int) (*tmdb.MediaDetail, error) {
	f.detailCalls++
	return f.detail, f.detailErr
}

func (f *fakeTMDB) Videos(context.Context, string, int) ([]tmdb.Video, error) {
	return f.videos, f.videosErr
}

func (f *fakeTMDB) SearchMedia(context.Context, string, string, int) (*tmdb.MediaPage, error) {
	return &tmdb.MediaPage{}, nil
}

func TestResolveSnapshot(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves detail and trailer", func(t *testing.T) {
		client := &fakeTMDB{
			detail: &tmdb.MediaDetail{
				Title: "Avengers: Doomsday", OriginalTitle: "Avengers: Doomsday",
				ReleaseDate: "2026-12-18", Overview: "…",
				PosterPath: "/p.jpg", BackdropPath: "/b.jpg",
				VoteAverage: 8.2, Runtime: 150, USCertification: "PG-13",
				IMDbID: "tt1", TVDBID: 42,
			},
			videos: []tmdb.Video{
				{Site: "youtube", Key: "teaser", Type: "Teaser"},
				{Site: "youtube", Key: "trailer1", Type: "Trailer", Official: true},
			},
		}

		got, err := resolveSnapshot(ctx, client, Input{MediaType: MediaMovie, TMDBID: 1})
		if err != nil {
			t.Fatalf("resolveSnapshot: %v", err)
		}
		if got.Title != "Avengers: Doomsday" || got.Year != 2026 {
			t.Fatalf("title/year = %q/%d", got.Title, got.Year)
		}
		if got.ReleaseDate == nil || got.ReleaseDate.Format("2006-01-02") != "2026-12-18" {
			t.Fatalf("release date = %v", got.ReleaseDate)
		}
		if got.TrailerSite != tmdb.SiteYouTube || got.TrailerKey != "trailer1" {
			t.Fatalf("trailer = %q/%q, want the official trailer", got.TrailerSite, got.TrailerKey)
		}
	})

	t.Run("no trailer is not an error", func(t *testing.T) {
		client := &fakeTMDB{detail: &tmdb.MediaDetail{Title: "Quiet", ReleaseDate: "2026-01-02"}}
		got, err := resolveSnapshot(ctx, client, Input{MediaType: MediaSeries, TMDBID: 2})
		if err != nil {
			t.Fatalf("resolveSnapshot: %v", err)
		}
		if got.TrailerSite != "" || got.TrailerKey != "" {
			t.Fatalf("trailer = %q/%q, want empty", got.TrailerSite, got.TrailerKey)
		}
	})

	t.Run("detail error is returned wrapped", func(t *testing.T) {
		client := &fakeTMDB{detailErr: errors.New("boom")}
		if _, err := resolveSnapshot(ctx, client, Input{MediaType: MediaMovie, TMDBID: 3}); err == nil {
			t.Fatal("resolveSnapshot: want an error")
		}
	})
}

func TestBestTrailerSelection(t *testing.T) {
	videos := []tmdb.Video{
		{Site: "youtube", Key: "a", Type: "Clip"},
		{Site: "vimeo", Key: "b", Type: "Trailer", Official: true},
		{Site: "youtube", Key: "c", Type: "Trailer"},
	}
	if site, key := tmdb.BestTrailer(videos); site != tmdb.SiteYouTube || key != "c" {
		t.Fatalf("BestTrailer = %q/%q, want youtube/c", site, key)
	}
	if site, key := tmdb.BestTrailer(nil); site != "" || key != "" {
		t.Fatalf("BestTrailer(nil) = %q/%q, want empty", site, key)
	}
}
