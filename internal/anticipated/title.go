// Package anticipated owns the administrator-curated list of upcoming movies and
// series the library does not have yet, shown as the "Most Anticipated" Home row
// and, when featured, as the Coming Soon hero.
//
// Nothing here stores a catalog content ID. A title that arrives in the library is
// matched by its provider IDs at read time, so catalog re-anchoring, merging and
// deleting never touch this data — the same invariant the external watchlist
// tables keep.
package anticipated

import (
	"fmt"
	"strings"
	"time"
)

// Media types a curated entry can have.
const (
	MediaMovie  = "movie"
	MediaSeries = "series"
)

// States an entry can be in, mirroring watchlist_titles.
const (
	StateActive      = "active"
	StateNeedsReview = "needs_review"
	StateRemoved     = "removed"
)

// MaxEntries bounds the curated list so the hero pool and the refresh run stay
// bounded.
const MaxEntries = 50

// Availability is how a curated title reads on the hero. It is decided at read
// time, never stored.
type Availability string

const (
	// AvailabilityUnreleased is a title whose release date is still in the future.
	AvailabilityUnreleased Availability = "unreleased"
	// AvailabilityReleasedUnowned is a title released but still absent from the
	// library.
	AvailabilityReleasedUnowned Availability = "released_unowned"
	// AvailabilityInLibrary is a title the library now owns; its slide plays.
	AvailabilityInLibrary Availability = "in_library"
)

// Title is one curated entry as stored.
type Title struct {
	ID            int64
	MediaType     string
	TMDBID        int
	IMDbID        string
	TVDBID        int
	Position      int
	Title         string
	OriginalTitle string
	Year          int
	ReleaseDate   *time.Time
	Overview      string
	// PosterPath, BackdropPath and LogoPath are TMDB path fragments, not
	// asset-store keys.
	PosterPath     string
	BackdropPath   string
	LogoPath       string
	VoteAverage    float64
	Runtime        int
	EpisodeRuntime int
	Certification  string
	TrailerSite    string
	TrailerKey     string
	State          string
	LastCheckedAt  *time.Time
}

// Input is one admin-supplied entry in a whole-list replace.
type Input struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
}

// Result is one TMDB search hit the curation picker shows, and the shape the
// admin search returns.
type Result struct {
	MediaType    string  `json:"media_type"`
	TMDBID       int     `json:"tmdb_id"`
	Title        string  `json:"title"`
	Year         int     `json:"year,omitempty"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	PosterPath   string  `json:"poster_path,omitempty"`
	BackdropPath string  `json:"backdrop_path,omitempty"`
	Overview     string  `json:"overview,omitempty"`
	VoteAverage  float64 `json:"vote_average,omitempty"`
}

// ValidateInputs checks a whole-list replace before anything is written: at most
// MaxEntries entries, each a known media type with a positive TMDB ID, and no
// duplicate (media_type, tmdb_id).
func ValidateInputs(in []Input) error {
	if len(in) > MaxEntries {
		return fmt.Errorf("anticipated: at most %d titles, got %d", MaxEntries, len(in))
	}
	seen := make(map[string]struct{}, len(in))
	for i, e := range in {
		mt := strings.TrimSpace(e.MediaType)
		if mt != MediaMovie && mt != MediaSeries {
			return fmt.Errorf("anticipated: entry %d: media_type must be %q or %q", i, MediaMovie, MediaSeries)
		}
		if e.TMDBID <= 0 {
			return fmt.Errorf("anticipated: entry %d: tmdb_id must be positive", i)
		}
		key := mt + ":" + fmt.Sprint(e.TMDBID)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("anticipated: entry %d: %s %d is listed twice", i, mt, e.TMDBID)
		}
		seen[key] = struct{}{}
	}
	return nil
}
