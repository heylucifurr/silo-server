package sections

import (
	"strings"

	"github.com/Silo-Server/silo-server/internal/models"
)

// Availability values an anticipated slide reports. They are additive on the
// card contract; the client branches on these rather than on the id prefix, so it
// never depends on the server's internal id convention.
const (
	AvailabilityInLibrary  = "in_library"
	AvailabilityUnreleased = "unreleased"
	// AvailabilityReleasedUnowned stays in the contract for a server that can
	// tell a curated title is already obtainable. This one never sends it: it
	// holds no date that says a copy can be had, so a curated title reads
	// unreleased until a copy arrives (see AnticipatedAvailability).
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
// hero: a real catalog item is in the library, and a curated title the library
// does not own is unreleased.
//
// A curated title stays unreleased after the release date in its snapshot on
// purpose. That date is when a title opened, not when anyone can own a copy, and
// a curated row is the administrator's list of what they are waiting for, so
// Coming soon holds until a copy arrives and the entry matches the library. A
// viewer who wants to ask for a title that is already out does it from the title
// page, which carries the request actions.
func AnticipatedAvailability(item *models.MediaItem) string {
	if item == nil || !IsExternalContentID(item.ContentID) {
		return AvailabilityInLibrary
	}
	return AvailabilityUnreleased
}
