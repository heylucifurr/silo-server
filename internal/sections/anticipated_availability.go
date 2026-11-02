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
