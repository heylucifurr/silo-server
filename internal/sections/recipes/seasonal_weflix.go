package recipes

import "time"

// WeFlix: holiday precedence for rows dressed for the web.
//
// Upstream ranks in-season built-in holidays by SeasonalThemeOrder alone, so
// a holiday whose window overlaps a higher-ranked one disappears for the
// overlap: Christmas moved to start on 11-05 gives way to Thanksgiving
// (11-22 to 11-30) and its web appearance with it. A holiday the admin has
// dressed with an appearance is one they want seen, so among the enabled
// holidays in season, the first dressed one in SeasonalThemeOrder wins. A row
// with no appearance on any in-season holiday keeps upstream's order exactly.
//
// Hooked in from ActiveSeasonalPick in seasonal.go (one line); keep the hook
// when merging upstream. See docs/features/seasonal-rows.md.

// weflixActiveTheme is ActiveTheme with dressed holidays first.
func (p SeasonalThemedParams) weflixActiveTheme(now time.Time) string {
	if len(p.EnabledThemes) == 0 {
		return ""
	}
	enabled := make(map[string]bool, len(p.EnabledThemes))
	for _, t := range p.EnabledThemes {
		enabled[t] = true
	}
	first := ""
	for _, t := range SeasonalThemeOrder {
		if !enabled[t] || !p.themeInSeason(t, now) {
			continue
		}
		if !usableAppearance(p.ThemeAppearance[t]).Empty() {
			return t
		}
		if first == "" {
			first = t
		}
	}
	return first
}
