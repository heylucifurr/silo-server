package apiv2

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
)

// A layout entry carries a seasonal row's web appearance under its own optional
// member, with the stored reference turned into the URL the image is fetched
// from; every other row answers exactly the members it always has.
func TestSectionLayoutCarriesSeasonalAppearance(t *testing.T) {
	const ref = "0123456789abcdef.webp"
	view := handlers.SectionLayoutView{Sections: []handlers.SectionLayoutEntryView{
		{
			ID: "seasonal", SectionType: "seasonal_themed", Title: "Christmas Favorites",
			Appearance: &handlers.SeasonalAppearanceView{
				BackgroundRef: ref,
				Headline:      "Come experience the magic",
				Subtitle:      "Your Hogwarts Adventure Awaits",
				ButtonLabel:   "Watch now",
			},
		},
		{ID: "recently_added", SectionType: "recently_added", Title: "Recently Added"},
	}}

	out := sectionLayoutOf(view)
	if len(out.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(out.Sections))
	}

	seasonal := out.Sections[0].Appearance
	if seasonal == nil {
		t.Fatal("the seasonal row lost its appearance")
	}
	if want := Prefix + "/seasonal-backgrounds/" + ref; seasonal.BackgroundURL != want {
		t.Errorf("background_url = %q, want %q", seasonal.BackgroundURL, want)
	}
	if seasonal.Headline != "Come experience the magic" || seasonal.Subtitle != "Your Hogwarts Adventure Awaits" || seasonal.ButtonLabel != "Watch now" {
		t.Errorf("appearance text = %+v", *seasonal)
	}
	if out.Sections[1].Appearance != nil {
		t.Errorf("an ordinary row gained an appearance: %+v", *out.Sections[1].Appearance)
	}

	// The wire shape: the member is optional and omitted when absent, so a
	// client that reads the layout it already knew sees the same body.
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"appearance"`, `"background_url"`, `"headline"`, `"subtitle"`, `"button_label"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("layout body is missing %s: %s", want, body)
		}
	}
	if strings.Count(string(body), `"appearance"`) != 1 {
		t.Errorf("appearance appears %d times, want once: %s", strings.Count(string(body), `"appearance"`), body)
	}
}
