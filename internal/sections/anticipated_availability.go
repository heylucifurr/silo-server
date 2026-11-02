package sections

import (
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
)

// Availability values an anticipated slide reports. They are additive on the
// card contract; the client branches on these rather than on the id prefix, so it
// never depends on the server's internal id convention.
const (
	AvailabilityInLibrary       = "in_library"
	AvailabilityUnreleased      = "unreleased"
	AvailabilityReleasedUnowned = "released_unowned"
)

// IsExternalContentID reports whether a section item id is a synthetic
// anticipated id rather than a catalog content id.
func IsExternalContentID(contentID string) bool {
	return strings.HasPrefix(contentID, ExternalContentIDPrefix)
}

// tmdbImageBase is TMDB's image CDN; an unowned anticipated title's artwork is
// served straight from it, not from the asset store.
const tmdbImageBase = "https://image.tmdb.org/t/p/"

// TMDBImageURL composes a browser-loadable URL from a TMDB path fragment. An
// empty path yields "", which a client reads as "no artwork".
func TMDBImageURL(path, size string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return tmdbImageBase + size + path
}

// AnticipatedAvailability reports how an anticipated section item reads on the
// hero at now: a real catalog item is in the library; a synthetic external item
// is unreleased until its release date has passed, then released_unowned. An
// entry with no release date reads as released_unowned (it is not unreleased).
func AnticipatedAvailability(item *models.MediaItem, now time.Time) string {
	if item == nil || !IsExternalContentID(item.ContentID) {
		return AvailabilityInLibrary
	}
	if item.ReleaseDate != nil {
		if date, err := time.Parse("2006-01-02", *item.ReleaseDate); err == nil && date.After(now) {
			return AvailabilityUnreleased
		}
	}
	return AvailabilityReleasedUnowned
}
