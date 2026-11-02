package anticipated

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// TMDB is the slice of the TMDB client the curation service reads.
// *tmdb.Client satisfies it.
type TMDB interface {
	GetMediaDetail(ctx context.Context, mediaType string, id int) (*tmdb.MediaDetail, error)
	Videos(ctx context.Context, mediaType string, id int) ([]tmdb.Video, error)
	SearchMedia(ctx context.Context, mediaType, query string, page int) (*tmdb.MediaPage, error)
}

// tmdbMediaType maps a Silo media type to the one TMDB's endpoints take.
func tmdbMediaType(mediaType string) string {
	if mediaType == MediaSeries {
		return "tv"
	}
	return "movie"
}

// resolveSnapshot reads a title's metadata and trailer from TMDB and returns it as
// a Title with no ID and Position 0 (the store assigns those). The trailer comes
// from TMDB videos because an unowned title has no item_videos row; a title with
// no usable trailer yields empty trailer fields and no error.
func resolveSnapshot(ctx context.Context, client TMDB, in Input) (Title, error) {
	mediaType := tmdbMediaType(in.MediaType)
	detail, err := client.GetMediaDetail(ctx, mediaType, in.TMDBID)
	if err != nil {
		return Title{}, fmt.Errorf("reading TMDB detail for %s %d: %w", in.MediaType, in.TMDBID, err)
	}
	if detail == nil {
		return Title{}, fmt.Errorf("reading TMDB detail for %s %d: no detail returned", in.MediaType, in.TMDBID)
	}

	videos, err := client.Videos(ctx, mediaType, in.TMDBID)
	if err != nil {
		return Title{}, fmt.Errorf("reading TMDB videos for %s %d: %w", in.MediaType, in.TMDBID, err)
	}
	site, key := tmdb.BestTrailer(videos)

	t := Title{
		MediaType:     in.MediaType,
		TMDBID:        in.TMDBID,
		IMDbID:        detail.IMDbID,
		TVDBID:        detail.TVDBID,
		Title:         detail.Title,
		OriginalTitle: detail.OriginalTitle,
		Year:          detail.Year,
		ReleaseDate:   parseDate(detail.ReleaseDate),
		Overview:      detail.Overview,
		PosterPath:    detail.PosterPath,
		BackdropPath:  detail.BackdropPath,
		VoteAverage:   detail.VoteAverage,
		Runtime:       detail.Runtime,
		Certification: detail.USCertification,
		TrailerSite:   site,
		TrailerKey:    key,
	}
	if t.Year == 0 && t.ReleaseDate != nil {
		t.Year = t.ReleaseDate.Year()
	}
	return t, nil
}

// parseDate reads a TMDB date (YYYY-MM-DD), returning nil for an empty or
// unparseable value.
func parseDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &t
}
