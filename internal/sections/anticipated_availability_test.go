package sections

import (
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
)

func TestAnticipatedAvailability(t *testing.T) {
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	future, past := "2026-12-18", "2026-01-01"

	cases := []struct {
		name string
		item *models.MediaItem
		want string
	}{
		{"owned catalog item", &models.MediaItem{ContentID: "tvdb-372264"}, AvailabilityInLibrary},
		{"unreleased external", &models.MediaItem{ContentID: ExternalContentID("movie", 550), ReleaseDate: &future}, AvailabilityUnreleased},
		{"released external", &models.MediaItem{ContentID: ExternalContentID("series", 1), ReleaseDate: &past}, AvailabilityReleasedUnowned},
		{"external with no date", &models.MediaItem{ContentID: ExternalContentID("movie", 2)}, AvailabilityReleasedUnowned},
		{"nil item reads as owned", nil, AvailabilityInLibrary},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnticipatedAvailability(tc.item, now); got != tc.want {
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
