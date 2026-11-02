package handlers

import (
	"testing"

	"github.com/Silo-Server/silo-server/internal/heroclips"
	"github.com/Silo-Server/silo-server/internal/sections"
)

// TestAnticipatedClipIDMatchesSlideID pins the one identity an unowned
// anticipated title has on both sides: the section item a hero renders and the
// clip that plays behind it. They are built in two packages, so nothing else
// stops the two strings drifting apart.
func TestAnticipatedClipIDMatchesSlideID(t *testing.T) {
	for _, tc := range []struct {
		mediaType string
		tmdbID    int
	}{
		{"movie", 1083381},
		{"series", 1399},
	} {
		if got, want := heroclips.AnticipatedCandidateID(tc.mediaType, tc.tmdbID),
			sections.ExternalContentID(tc.mediaType, tc.tmdbID); got != want {
			t.Fatalf("clip id %q != slide id %q", got, want)
		}
	}
}
