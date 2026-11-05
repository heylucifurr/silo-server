package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/blobstore/blobstoretest"
	"github.com/Silo-Server/silo-server/internal/sections"
	"github.com/Silo-Server/silo-server/internal/seasonalbackground"
)

func seasonalBackgroundHandler(t *testing.T) (*SeasonalBackgroundHandler, *blobstoretest.Memory) {
	t.Helper()
	store := blobstoretest.New()
	return NewSeasonalBackgroundHandler(seasonalbackground.NewService(store)), store
}

func seasonalBackgroundErrorStatus(t *testing.T, err error) int {
	t.Helper()
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("error %v is not an *APIError", err)
	}
	return apiErr.Status
}

// A process without a blob store refuses an upload rather than dropping it, and
// an upload the server cannot draw is a field error. Neither reaches a store.
func TestSeasonalBackgroundUploadRefusals(t *testing.T) {
	h, store := seasonalBackgroundHandler(t)
	if _, err := NewSeasonalBackgroundHandler(seasonalbackground.NewService(nil)).UploadSeasonalBackground(t.Context(), "image/png", []byte("x")); seasonalBackgroundErrorStatus(t, err) != http.StatusServiceUnavailable {
		t.Errorf("no storage: err = %v, want 503", err)
	}

	_, err := h.UploadSeasonalBackground(t.Context(), "application/pdf", []byte("not an image"))
	if status := seasonalBackgroundErrorStatus(t, err); status != http.StatusBadRequest {
		t.Errorf("non-image: status = %d, want 400", status)
	}
	apiErr, _ := errors.AsType[*APIError](err)
	if apiErr.Field != "file" {
		t.Errorf("field = %q, want \"file\"", apiErr.Field)
	}
	if len(store.Objects) != 0 {
		t.Errorf("a refused upload stored %d objects", len(store.Objects))
	}
}

// Only a reference the server wrote is read, and a stored one comes back as a
// WebP.
func TestSeasonalBackgroundRead(t *testing.T) {
	h, store := seasonalBackgroundHandler(t)
	ref := "0123456789abcdef.webp"
	if _, _, err := h.ReadSeasonalBackground(t.Context(), "../../secret.webp"); seasonalBackgroundErrorStatus(t, err) != http.StatusNotFound {
		t.Errorf("foreign reference: err = %v, want 404", err)
	}
	if _, _, err := h.ReadSeasonalBackground(t.Context(), ref); seasonalBackgroundErrorStatus(t, err) != http.StatusNotFound {
		t.Errorf("absent object: err = %v, want 404", err)
	}

	if err := store.Put(t.Context(), seasonalbackground.Key(ref), []byte("webp-bytes")); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	data, media, err := h.ReadSeasonalBackground(t.Context(), ref)
	if err != nil {
		t.Fatalf("ReadSeasonalBackground: %v", err)
	}
	if string(data) != "webp-bytes" || media != "image/webp" {
		t.Errorf("read %q as %q, want the stored bytes as image/webp", data, media)
	}
}

// seasonalAppearanceOf is the join between a row's saved config and what the
// web draws: the appearance of the holiday in season, and nothing at all for
// any other row.
func TestSeasonalAppearanceOf(t *testing.T) {
	oct14 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	config := func(t *testing.T, raw string) json.RawMessage {
		t.Helper()
		if !json.Valid([]byte(raw)) {
			t.Fatalf("test config is not JSON: %s", raw)
		}
		return json.RawMessage(raw)
	}
	halloweenRow := config(t, `{"enabled_themes":["halloween"],"theme_appearance":{"halloween":{"background":"0123456789abcdef.webp","headline":"Spooky season","subtitle":"Family picks","button_label":"Watch now"}}}`)
	christmasRow := config(t, `{"enabled_themes":["christmas"],"theme_appearance":{"christmas":{"headline":"Merry Christmas"}}}`)

	cases := []struct {
		name    string
		section sections.ResolvedSection
		want    *SeasonalAppearanceView
	}{
		{
			name:    "the in-season holiday's appearance",
			section: sections.ResolvedSection{SectionType: sections.SectionSeasonalThemed, Config: halloweenRow},
			want:    &SeasonalAppearanceView{BackgroundRef: "0123456789abcdef.webp", Headline: "Spooky season", Subtitle: "Family picks", ButtonLabel: "Watch now"},
		},
		{
			name:    "an off-season row dresses as nothing",
			section: sections.ResolvedSection{SectionType: sections.SectionSeasonalThemed, Config: christmasRow},
		},
		{
			name:    "another row type has no appearance",
			section: sections.ResolvedSection{SectionType: sections.SectionRecentlyAdded, Config: halloweenRow},
		},
		{
			name:    "a row with no config has no appearance",
			section: sections.ResolvedSection{SectionType: sections.SectionSeasonalThemed},
		},
		{
			name:    "a config that is not the seasonal shape has no appearance",
			section: sections.ResolvedSection{SectionType: sections.SectionSeasonalThemed, Config: json.RawMessage(`"not an object"`)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := seasonalAppearanceOf(tc.section, oct14)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("appearance = %+v, want %+v", got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("appearance = %+v, want %+v", *got, *tc.want)
			}
		})
	}
}
