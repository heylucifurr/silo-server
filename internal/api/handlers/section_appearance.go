package handlers

import (
	"encoding/json"
	"time"

	"github.com/Silo-Server/silo-server/internal/sections"
	"github.com/Silo-Server/silo-server/internal/sections/recipes"
)

// SeasonalAppearanceView is how a seasonal row dresses itself on the web home
// page right now. The apps never read it: it rides a layout response the web
// client consumes, and the frozen v1 body does not carry it at all (see
// resolvedSectionLayoutResponse).
type SeasonalAppearanceView struct {
	// BackgroundRef is the content reference of the uploaded background, or
	// empty. The v2 listener turns it into a URL.
	BackgroundRef string
	Headline      string
	Subtitle      string
	ButtonLabel   string
}

// Empty reports whether the view says nothing worth sending.
func (v SeasonalAppearanceView) Empty() bool {
	return v.BackgroundRef == "" && v.Headline == "" && v.Subtitle == "" && v.ButtonLabel == ""
}

// seasonalAppearanceOf reports how a resolved row should dress itself at now,
// or nil for a row that is not a seasonal row or has no holiday in season. It
// re-reads the row's config the same way the seasonal fetch path does, so the
// background the web draws is the one belonging to the holiday the row is
// showing.
func seasonalAppearanceOf(s sections.ResolvedSection, now time.Time) *SeasonalAppearanceView {
	if s.SectionType != sections.SectionSeasonalThemed || len(s.Config) == 0 {
		return nil
	}
	var params recipes.SeasonalThemedParams
	if err := json.Unmarshal(s.Config, &params); err != nil {
		return nil
	}
	pick, ok := recipes.ActiveSeasonalPick(params, now)
	if !ok || pick.Appearance.Empty() {
		return nil
	}
	view := SeasonalAppearanceView{
		BackgroundRef: pick.Appearance.Background,
		Headline:      pick.Appearance.Headline,
		Subtitle:      pick.Appearance.Subtitle,
		ButtonLabel:   pick.Appearance.ButtonLabel,
	}
	if view.Empty() {
		return nil
	}
	return &view
}
