package recipes

import (
	"encoding/json"
	"testing"
	"time"
)

// TestWeflixSeasonalRowPick pins which holiday a Seasonal Picks row shows, and
// with what appearance, for the cases WeFlix relies on: its own window, the
// default windows, the New Year's window across the year end, and a dressed
// holiday winning an overlap with a higher-ranked undressed one (the
// Christmas-from-11-05 row that went blank when Thanksgiving opened on 11-22).
func TestWeflixSeasonalRowPick(t *testing.T) {
	look := SeasonalAppearance{
		Background:  "0123456789abcdef.webp",
		Headline:    "Unwrap the season's best",
		Subtitle:    "Your favorite Christmas movies await",
		ButtonLabel: "Ho Ho Ho!",
	}
	// The default row's holidays, as a newly created Seasonal Picks row saves them.
	var defaults SeasonalThemedParams
	if err := json.Unmarshal([]byte(DefaultSeasonalConfig), &defaults); err != nil {
		t.Fatal(err)
	}
	dressedChristmas := defaults
	dressedChristmas.ThemeDates = map[string]SeasonalDates{"christmas": {Start: "11-05", End: "12-25"}}
	dressedChristmas.ThemeAppearance = map[string]SeasonalAppearance{"christmas": look}
	undressedChristmas := dressedChristmas
	undressedChristmas.ThemeAppearance = nil

	at := func(month time.Month, day, hour int) time.Time {
		return time.Date(2026, month, day, hour, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name      string
		p         SeasonalThemedParams
		now       time.Time
		wantTheme string // "" means nothing in season
		wantLook  SeasonalAppearance
	}{
		{name: "own window: inside", p: dressedChristmas, now: at(time.November, 10, 12), wantTheme: "christmas", wantLook: look},
		{name: "own window: dressed holiday wins the Thanksgiving overlap", p: dressedChristmas, now: at(time.November, 25, 12), wantTheme: "christmas", wantLook: look},
		{name: "own window: undressed overlap keeps upstream's order", p: undressedChristmas, now: at(time.November, 25, 12), wantTheme: "thanksgiving"},
		{name: "own window: last day", p: dressedChristmas, now: at(time.December, 25, 20), wantTheme: "christmas", wantLook: look},
		{name: "own window: outside, after the end and its grace", p: dressedChristmas, now: at(time.December, 26, 18), wantTheme: "new_year"},
		{name: "own window: outside, before the start", p: SeasonalThemedParams{EnabledThemes: []string{"christmas"}, ThemeDates: dressedChristmas.ThemeDates, ThemeAppearance: dressedChristmas.ThemeAppearance}, now: at(time.November, 4, 12)},
		{name: "default dates when none are set: Christmas in December", p: SeasonalThemedParams{EnabledThemes: []string{"christmas"}}, now: at(time.December, 10, 12), wantTheme: "christmas"},
		{name: "default dates when none are set: Christmas not in November", p: SeasonalThemedParams{EnabledThemes: []string{"christmas"}}, now: at(time.November, 10, 12)},
		{name: "New Year's wraps the year end: Dec 31", p: SeasonalThemedParams{EnabledThemes: []string{"new_year"}}, now: at(time.December, 31, 12), wantTheme: "new_year"},
		{name: "New Year's wraps the year end: Jan 1", p: SeasonalThemedParams{EnabledThemes: []string{"new_year"}}, now: time.Date(2027, time.January, 1, 12, 0, 0, 0, time.UTC), wantTheme: "new_year"},
		{name: "New Year's wraps the year end: Jan 2 evening is out", p: SeasonalThemedParams{EnabledThemes: []string{"new_year"}}, now: time.Date(2027, time.January, 2, 18, 0, 0, 0, time.UTC)},
		{name: "New Year's wraps the year end: Dec 25 is out", p: SeasonalThemedParams{EnabledThemes: []string{"new_year"}}, now: at(time.December, 25, 12)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pick, ok := ActiveSeasonalPick(tc.p, tc.now)
			if ok != (tc.wantTheme != "") || pick.Theme != tc.wantTheme {
				t.Fatalf("pick = %q (ok=%v), want %q", pick.Theme, ok, tc.wantTheme)
			}
			if pick.Appearance != tc.wantLook {
				t.Fatalf("appearance = %+v, want %+v", pick.Appearance, tc.wantLook)
			}
		})
	}
}
