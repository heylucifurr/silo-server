package sections

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Silo-Server/silo-server/internal/anticipated"
	"github.com/Silo-Server/silo-server/internal/catalog"
	"github.com/Silo-Server/silo-server/internal/models"
)

// ExternalContentIDPrefix marks a section item that is not a catalog row: an
// anticipated title the library does not own yet. The remainder is
// "<media_type>-<tmdb_id>". The handler, the clip resolver and the clients all
// key off this prefix or the availability the handler derives from it.
const ExternalContentIDPrefix = "ext:"

// ExternalContentID builds the synthetic content id of an unowned entry.
func ExternalContentID(mediaType string, tmdbID int) string {
	return ExternalContentIDPrefix + mediaType + "-" + strconv.Itoa(tmdbID)
}

// anticipatedTitleReader reads the admin-curated list. Satisfied by
// *anticipated.Store. Nil renders the row empty.
type anticipatedTitleReader interface {
	List(ctx context.Context) ([]anticipated.Title, error)
}

// fetchAnticipated reads the curated list in admin order and resolves each entry:
// a title the library owns becomes its catalog item (so the slide plays), and one
// it does not becomes a synthetic external item carrying the TMDB snapshot.
func (f *Fetcher) fetchAnticipated(ctx context.Context, s ResolvedSection, libraryID *int, libraryIDs []int, filter catalog.AccessFilter) ([]*models.MediaItem, int, error) {
	if f.Anticipated == nil {
		return []*models.MediaItem{}, 0, nil
	}
	entries, err := f.Anticipated.List(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("listing anticipated titles: %w", err)
	}
	if len(entries) == 0 {
		return []*models.MediaItem{}, 0, nil
	}

	// Match the entries to library items, one lookup per media type.
	movieIDs := make([]string, 0, len(entries))
	seriesIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		id := strconv.Itoa(e.TMDBID)
		if e.MediaType == anticipated.MediaSeries {
			seriesIDs = append(seriesIDs, id)
		} else {
			movieIDs = append(movieIDs, id)
		}
	}
	var movieLookup, seriesLookup *catalog.ExternalIDLookup
	if f.AnticipatedResolver != nil {
		if len(movieIDs) > 0 {
			movieLookup, err = f.AnticipatedResolver.GetByExternalIDs(ctx, catalog.ExternalIDBatch{TMDBIDs: movieIDs}, "movie")
			if err != nil {
				return nil, 0, fmt.Errorf("resolving anticipated movies: %w", err)
			}
		}
		if len(seriesIDs) > 0 {
			seriesLookup, err = f.AnticipatedResolver.GetByExternalIDs(ctx, catalog.ExternalIDBatch{TMDBIDs: seriesIDs}, "series")
			if err != nil {
				return nil, 0, fmt.Errorf("resolving anticipated series: %w", err)
			}
		}
	}

	// Owned entries: load their catalog items in one call so access filtering,
	// artwork and play targets behave exactly as for any other row.
	ownedIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		if contentID := resolveAnticipatedContentID(movieLookup, seriesLookup, e); contentID != "" {
			ownedIDs = append(ownedIDs, contentID)
		}
	}
	ownedByID := map[string]*models.MediaItem{}
	if len(ownedIDs) > 0 {
		owned, err := f.fetchItemsByContentIDs(ctx, ownedIDs, libraryID, libraryIDs, filter)
		if err != nil {
			return nil, 0, fmt.Errorf("loading anticipated library items: %w", err)
		}
		for _, item := range owned {
			ownedByID[item.ContentID] = item
		}
	}

	limit := s.ItemLimit
	if limit <= 0 {
		limit = 20
	}

	out := make([]*models.MediaItem, 0, min(limit, len(entries)))
	for _, e := range entries {
		contentID := resolveAnticipatedContentID(movieLookup, seriesLookup, e)
		if contentID != "" {
			if item, ok := ownedByID[contentID]; ok {
				out = append(out, item)
			} else {
				continue // matched but filtered out for this viewer
			}
		} else {
			out = append(out, externalMediaItem(e))
		}
		if len(out) == limit {
			break
		}
	}
	return out, len(out), nil
}

// resolveAnticipatedContentID returns the library content id an entry owns, or ""
// when the library does not have it.
func resolveAnticipatedContentID(movieLookup, seriesLookup *catalog.ExternalIDLookup, e anticipated.Title) string {
	key := strconv.Itoa(e.TMDBID)
	if e.MediaType == anticipated.MediaSeries {
		if seriesLookup != nil {
			return seriesLookup.ByTMDB[key]
		}
		return ""
	}
	if movieLookup != nil {
		return movieLookup.ByTMDB[key]
	}
	return ""
}

// externalMediaItem builds the synthetic item for an entry the library lacks.
// PosterPath and BackdropPath are TMDB path fragments, not asset-store keys; the
// handler serves them from TMDB's image CDN.
func externalMediaItem(e anticipated.Title) *models.MediaItem {
	item := &models.MediaItem{
		ContentID:     ExternalContentID(e.MediaType, e.TMDBID),
		Type:          e.MediaType,
		Title:         e.Title,
		OriginalTitle: e.OriginalTitle,
		Year:          e.Year,
		Overview:      e.Overview,
		PosterPath:    e.PosterPath,
		BackdropPath:  e.BackdropPath,
		LogoPath:      e.LogoPath,
		ContentRating: e.Certification,
		Runtime:       e.Runtime,
		TmdbID:        strconv.Itoa(e.TMDBID),
		ImdbID:        e.IMDbID,
	}
	if e.ReleaseDate != nil {
		date := e.ReleaseDate.Format("2006-01-02")
		item.ReleaseDate = &date
	}
	if e.VoteAverage > 0 {
		vote := e.VoteAverage
		item.RatingTMDB = &vote
	}
	return item
}
