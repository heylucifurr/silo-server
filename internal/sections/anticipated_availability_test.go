package sections

import (
	"testing"

	"github.com/Silo-Server/silo-server/internal/models"
)

func TestAnticipatedAvailability(t *testing.T) {
	future, past := "2026-12-18", "2026-01-01"

	cases := []struct {
		name string
		item *models.MediaItem
		want string
	}{
		{"owned catalog item", &models.MediaItem{ContentID: "tvdb-372264"}, AvailabilityInLibrary},
		{"external before its release date", &models.MediaItem{ContentID: ExternalContentID("movie", 550), ReleaseDate: &future}, AvailabilityUnreleased},
		// The snapshot's release date is when a title opened, not when a copy can
		// be owned, so a curated title the library lacks reads Coming soon
		// whatever date it carries.
		{"external past its release date", &models.MediaItem{ContentID: ExternalContentID("series", 1), ReleaseDate: &past}, AvailabilityUnreleased},
		{"external with no date", &models.MediaItem{ContentID: ExternalContentID("movie", 2)}, AvailabilityUnreleased},
		{"nil item reads as owned", nil, AvailabilityInLibrary},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnticipatedAvailability(tc.item); got != tc.want {
				t.Fatalf("AnticipatedAvailability = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExternalContentIDIdentity(t *testing.T) {
	if got := ExternalContentID("movie", 550); got != "ext:movie-550" {
		t.Fatalf("ExternalContentID = %q", got)
	}
	if !IsExternalContentID("ext:series-1399") {
		t.Fatal("IsExternalContentID(ext:series-1399) = false")
	}
	if IsExternalContentID("tvdb-372264") {
		t.Fatal("a catalog id must not read as external")
	}
}
