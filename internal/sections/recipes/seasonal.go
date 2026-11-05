package recipes

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Silo-Server/silo-server/internal/collectionutil"
)

// DatePredicate is a function that reports whether a given time falls within
// the seasonal window.
type DatePredicate func(time.Time) bool

// seasonEndGrace keeps a dated season up this long after its last day ends.
// Days are the server's (UTC by default), so without it Halloween would end
// on the evening of October 31 for viewers in the Americas. On a server set
// to the viewer's own zone, the season runs into the next morning instead.
const seasonEndGrace = 12 * time.Hour

// inYearlyWindow reports whether t falls between start and end (MM-DD,
// inclusive), or within seasonEndGrace after end. An end before the start
// wraps across New Year. An unparseable bound never matches.
func inYearlyWindow(t time.Time, start, end string) bool {
	return onYearlyWindowDay(t, start, end) || onYearlyWindowDay(t.Add(-seasonEndGrace), start, end)
}

// onYearlyWindowDay reports whether t's date falls between start and end
// (MM-DD, inclusive).
func onYearlyWindowDay(t time.Time, start, end string) bool {
	sm, sd, ok1 := parseMonthDay(start)
	em, ed, ok2 := parseMonthDay(end)
	if !ok1 || !ok2 {
		return false
	}
	at := int(t.Month())*100 + t.Day()
	from, to := int(sm)*100+sd, int(em)*100+ed
	if from <= to {
		return at >= from && at <= to
	}
	return at >= from || at <= to
}

// weekdayBeforeHour returns a predicate that matches times on the given weekday
// strictly before the given hour (24-hour, e.g. 13 for 1pm).
func weekdayBeforeHour(weekday time.Weekday, hour int) DatePredicate {
	return func(t time.Time) bool {
		return t.Weekday() == weekday && t.Hour() < hour
	}
}

// weekdaysFromHour returns a predicate that matches times on any of the given
// weekdays at or after the given hour (24-hour, e.g. 17 for 5pm).
func weekdaysFromHour(hour int, weekdays ...time.Weekday) DatePredicate {
	set := make(map[time.Weekday]bool, len(weekdays))
	for _, d := range weekdays {
		set[d] = true
	}
	return func(t time.Time) bool {
		return set[t.Weekday()] && t.Hour() >= hour
	}
}

// easterWindow matches Palm Sunday through Easter Monday, and seasonEndGrace
// after it.
func easterWindow(t time.Time) bool {
	return onEasterWindowDay(t) || onEasterWindowDay(t.Add(-seasonEndGrace))
}

// onEasterWindowDay reports whether t's date is Palm Sunday through Easter
// Monday.
func onEasterWindowDay(t time.Time) bool {
	easter := easterSunday(t.Year(), t.Location())
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return !day.Before(easter.AddDate(0, 0, -7)) && !day.After(easter.AddDate(0, 0, 1))
}

// easterSunday returns Western Easter for year (the anonymous Gregorian
// algorithm).
func easterSunday(year int, loc *time.Location) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
}

// SeasonalPredicates maps theme names to their default date predicates: the
// catalog's Start-End window, or the theme's own rule. Every SeasonalThemes
// entry has one.
var SeasonalPredicates = func() map[string]DatePredicate {
	rules := map[string]DatePredicate{
		ThemeEaster:           easterWindow,
		ThemeSaturdayMorning:  weekdayBeforeHour(time.Saturday, 13),
		ThemeFamilyMovieNight: weekdaysFromHour(17, time.Friday, time.Saturday),
	}
	for _, theme := range SeasonalThemes {
		if theme.Start != "" {
			start, end := theme.Start, theme.End
			rules[theme.Key] = func(t time.Time) bool { return inYearlyWindow(t, start, end) }
		}
	}
	return rules
}()

// SeasonalThemeOrder lists the built-in themes from most to least specific.
// When several enabled themes are in season at once (a Saturday morning in
// October), the earlier one wins: the short holidays first, then the
// month-long ones, the weekly themes, and summer last. Custom holidays
// outrank all of them. Every SeasonalThemes entry appears exactly once.
var SeasonalThemeOrder = []string{
	ThemeValentines,
	ThemeStPatricks,
	ThemeEaster,
	ThemeThanksgiving,
	ThemeNewYear,
	ThemeChristmas,
	ThemeHalloween,
	ThemeSaturdayMorning,
	ThemeFamilyMovieNight,
	ThemeSummerBlockbuster,
	ThemeSummer,
}

// Seasonal row sources. The default ranks the vibe's list titles the library
// owns by IMDb popularity, then fills the row with the library's closest
// matches by recommendation embedding when the server has them. Lists only
// skips that fill.
const (
	SeasonalSourceRecommendations = "recommendations"
	SeasonalSourceLists           = "lists"
)

// MaxSeasonalCustomHolidays bounds the custom holidays one row may carry.
const MaxSeasonalCustomHolidays = 12

// SeasonalCustomHoliday is a holiday a user defines for one row: a yearly
// window (MM-DD, inclusive; End before Start wraps across New Year) and the
// public MDBList list whose titles it shows.
type SeasonalCustomHoliday struct {
	Name    string `json:"name"`
	Start   string `json:"start"`
	End     string `json:"end"`
	ListURL string `json:"list_url"`
	// Title names the row in season; empty uses Name.
	Title string `json:"title,omitempty"`
	// Appearance dresses the row on the web home page while this holiday is in
	// season. Absent on every row saved before it existed.
	Appearance SeasonalAppearance `json:"appearance,omitempty"`
}

// SeasonalDates is a yearly window a row sets for a built-in theme in place of
// its default: MM-DD, inclusive; End before Start wraps across New Year.
type SeasonalDates struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// SeasonalAppearance is how a seasonal row dresses itself on the web home page
// while one holiday is in season: an uploaded background, a headline, a
// subtitle and the label of the button that opens the row's catalog page. The
// apps read none of it — they show the row as they always have — so every field
// is optional and an all-empty value is what every row saved before this
// existed carries.
type SeasonalAppearance struct {
	// Background is the content reference of an uploaded background image
	// ("<hash>.webp"), stored under the seasonal backgrounds key prefix. Empty
	// means the web uses the first title's backdrop instead.
	Background  string `json:"background,omitempty"`
	Headline    string `json:"headline,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	ButtonLabel string `json:"button_label,omitempty"`
}

// Empty reports whether the appearance says nothing, so a row need not save an
// empty object for every holiday it enables.
func (a SeasonalAppearance) Empty() bool {
	return a.Background == "" && a.Headline == "" && a.Subtitle == "" && a.ButtonLabel == ""
}

// Trimmed returns the appearance with surrounding whitespace removed, which is
// what the row stores and serves. A button without a label keeps its empty
// value: "Watch now" is the web client's default, not a stored one, so a
// translated client can pick its own wording.
func (a SeasonalAppearance) Trimmed() SeasonalAppearance {
	a.Background = strings.TrimSpace(a.Background)
	a.Headline = strings.TrimSpace(a.Headline)
	a.Subtitle = strings.TrimSpace(a.Subtitle)
	a.ButtonLabel = strings.TrimSpace(a.ButtonLabel)
	return a
}

// Limits on the appearance's text, in runes. They are generous for the ~30% of
// the home page the text block occupies; the point is that a config cannot
// carry a novel into every home layout response.
const (
	MaxSeasonalHeadlineRunes    = 120
	MaxSeasonalSubtitleRunes    = 200
	MaxSeasonalButtonLabelRunes = 40
)

// SeasonalBackgroundRefPattern is the shape of a stored background reference: a
// truncated SHA-256 of the stored bytes and a WebP extension. Anything else is
// a hand-edited config, and it is dropped rather than turned into a storage key.
var SeasonalBackgroundRefPattern = regexp.MustCompile(`^[0-9a-f]{16}\.webp$`)

// SeasonalThemedParams configures the seasonal_themed resolver.
//
// There are two modes of use:
//
//  1. Multi-theme auto-cycle (preferred): set EnabledThemes to the themes the
//     row rotates through and optionally CustomHolidays. The fetcher picks the
//     first custom holiday in season, else the first SeasonalThemeOrder entry
//     whose predicate matches now; if none match, the section is empty
//     (suppressed off-season).
//
//  2. Legacy single-theme: set Theme to a single theme key. Mode controls
//     whether the section is hidden off-season (auto, default) or pinned.
//     Retained for backward compat with sections saved before the multi-theme
//     shape.
//
// Vibes picks a vibe per theme key; a missing or unknown entry uses the
// theme's default vibe. ThemeTitles optionally renames the row while a theme
// is in season; without an entry the row takes the vibe's title. ThemeDates
// moves a non-weekly theme's window; without an entry the theme keeps its
// default dates (Easter keeps moving with the calendar).
type SeasonalThemedParams struct {
	EnabledThemes  []string                 `json:"enabled_themes,omitempty"`
	ThemeTitles    map[string]string        `json:"theme_titles,omitempty"`
	Vibes          map[string]string        `json:"vibes,omitempty"`
	ThemeDates     map[string]SeasonalDates `json:"theme_dates,omitempty"`
	Source         string                   `json:"source,omitempty"`
	CustomHolidays []SeasonalCustomHoliday  `json:"custom_holidays,omitempty"`
	// ThemeAppearance dresses the row on the web home page while a built-in
	// holiday is in season, keyed by theme. Custom holidays carry their own on
	// the holiday.
	ThemeAppearance map[string]SeasonalAppearance `json:"theme_appearance,omitempty"`

	// Legacy single-theme fields. Still honored when EnabledThemes and
	// CustomHolidays are empty.
	Theme string `json:"theme,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

// UsesListsOnly reports whether the row skips the recommendation fill.
func (p SeasonalThemedParams) UsesListsOnly() bool { return p.Source == SeasonalSourceLists }

// themeInSeason reports whether a built-in theme is in season at now, using
// the row's own dates for it when set.
func (p SeasonalThemedParams) themeInSeason(theme string, now time.Time) bool {
	if dates, ok := p.ThemeDates[theme]; ok {
		return inYearlyWindow(now, dates.Start, dates.End)
	}
	pred, ok := SeasonalPredicates[theme]
	return ok && pred(now)
}

var validSeasonalModes = map[string]bool{
	"":       true, // valid; treated as "auto"
	"auto":   true,
	"pinned": true,
}

var validSeasonalSources = map[string]bool{
	"":                            true, // treated as recommendations
	SeasonalSourceRecommendations: true,
	SeasonalSourceLists:           true,
}

type seasonalRecipe struct{}

func (seasonalRecipe) Type() string                   { return "seasonal_themed" }
func (seasonalRecipe) NewParams() any                 { return &SeasonalThemedParams{} }
func (seasonalRecipe) DefaultCacheTTL() time.Duration { return 24 * time.Hour }

func (seasonalRecipe) Resolve(rc ResolverContext) (ResolvedItems, error) {
	return delegateResolve("seasonal_themed", rc)
}

func (seasonalRecipe) Validate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("seasonal_themed: enabled_themes, custom_holidays or theme is required")
	}
	var p SeasonalThemedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if !validSeasonalModes[p.Mode] {
		return fmt.Errorf("seasonal_themed: invalid mode %q (want auto|pinned)", p.Mode)
	}
	if !validSeasonalSources[p.Source] {
		return fmt.Errorf("seasonal_themed: invalid source %q (want recommendations|lists)", p.Source)
	}
	for theme, vibe := range p.Vibes {
		entry, ok := SeasonalThemeByKey(theme)
		if !ok {
			return fmt.Errorf("seasonal_themed: unknown theme %q in vibes", theme)
		}
		if !entry.hasVibe(vibe) {
			return fmt.Errorf("seasonal_themed: unknown vibe %q for theme %q", vibe, theme)
		}
	}
	for theme, dates := range p.ThemeDates {
		entry, ok := SeasonalThemeByKey(theme)
		if !ok {
			return fmt.Errorf("seasonal_themed: unknown theme %q in theme_dates", theme)
		}
		if entry.Weekly {
			return fmt.Errorf("seasonal_themed: theme %q recurs weekly; its dates cannot change", theme)
		}
		if _, _, ok := parseMonthDay(dates.Start); !ok {
			return fmt.Errorf("seasonal_themed: theme %q: start %q is not MM-DD", theme, dates.Start)
		}
		if _, _, ok := parseMonthDay(dates.End); !ok {
			return fmt.Errorf("seasonal_themed: theme %q: end %q is not MM-DD", theme, dates.End)
		}
	}
	for theme, appearance := range p.ThemeAppearance {
		if _, ok := SeasonalThemeByKey(theme); !ok {
			return fmt.Errorf("seasonal_themed: unknown theme %q in theme_appearance", theme)
		}
		if err := validateAppearance(appearance); err != nil {
			return fmt.Errorf("seasonal_themed: theme %q: %w", theme, err)
		}
	}
	if err := validateCustomHolidays(p.CustomHolidays); err != nil {
		return err
	}
	// Multi-theme mode wins when populated.
	if len(p.EnabledThemes) > 0 || len(p.CustomHolidays) > 0 {
		for _, t := range p.EnabledThemes {
			if _, ok := SeasonalPredicates[t]; !ok {
				return fmt.Errorf("seasonal_themed: unknown theme %q in enabled_themes", t)
			}
		}
		return nil
	}
	// Legacy single-theme path.
	if p.Theme == "" {
		return errors.New("seasonal_themed: enabled_themes, custom_holidays or theme is required")
	}
	if _, ok := SeasonalPredicates[p.Theme]; !ok {
		return fmt.Errorf("seasonal_themed: unknown theme %q", p.Theme)
	}
	return nil
}

func validateCustomHolidays(holidays []SeasonalCustomHoliday) error {
	if len(holidays) > MaxSeasonalCustomHolidays {
		return fmt.Errorf("seasonal_themed: at most %d custom holidays", MaxSeasonalCustomHolidays)
	}
	for i, h := range holidays {
		if strings.TrimSpace(h.Name) == "" {
			return fmt.Errorf("seasonal_themed: custom holiday %d needs a name", i+1)
		}
		if utf8.RuneCountInString(h.Name) > 80 || utf8.RuneCountInString(h.Title) > 80 {
			return fmt.Errorf("seasonal_themed: custom holiday %q: name and title are at most 80 characters", h.Name)
		}
		if _, _, ok := parseMonthDay(h.Start); !ok {
			return fmt.Errorf("seasonal_themed: custom holiday %q: start %q is not MM-DD", h.Name, h.Start)
		}
		if _, _, ok := parseMonthDay(h.End); !ok {
			return fmt.Errorf("seasonal_themed: custom holiday %q: end %q is not MM-DD", h.Name, h.End)
		}
		if _, err := collectionutil.CanonicalMDBListURL(h.ListURL); err != nil {
			return fmt.Errorf("seasonal_themed: custom holiday %q: list_url must be an MDBList list (https://mdblist.com/lists/...)", h.Name)
		}
		if err := validateAppearance(h.Appearance); err != nil {
			return fmt.Errorf("seasonal_themed: custom holiday %q: %w", h.Name, err)
		}
	}
	return nil
}

// validateAppearance refuses an appearance the server could not serve or the
// web could not draw: a background reference it did not write, or text well
// past what the row's text block holds.
func validateAppearance(a SeasonalAppearance) error {
	if a.Background != "" && !SeasonalBackgroundRefPattern.MatchString(strings.TrimSpace(a.Background)) {
		return errors.New("background must be a reference an upload returned")
	}
	if utf8.RuneCountInString(a.Headline) > MaxSeasonalHeadlineRunes {
		return fmt.Errorf("headline is at most %d characters", MaxSeasonalHeadlineRunes)
	}
	if utf8.RuneCountInString(a.Subtitle) > MaxSeasonalSubtitleRunes {
		return fmt.Errorf("subtitle is at most %d characters", MaxSeasonalSubtitleRunes)
	}
	if utf8.RuneCountInString(a.ButtonLabel) > MaxSeasonalButtonLabelRunes {
		return fmt.Errorf("button_label is at most %d characters", MaxSeasonalButtonLabelRunes)
	}
	return nil
}

// parseMonthDay parses MM-DD. Feb 29 is allowed; in other years it never
// matches on its own, so a window that starts or ends there still works.
func parseMonthDay(s string) (time.Month, int, bool) {
	if len(s) != 5 || s[2] != '-' {
		return 0, 0, false
	}
	month, errMonth := strconv.Atoi(s[:2])
	day, errDay := strconv.Atoi(s[3:])
	if errMonth != nil || errDay != nil {
		return 0, 0, false
	}
	if month < 1 || month > 12 || day < 1 {
		return 0, 0, false
	}
	if day > time.Date(2024, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return 0, 0, false
	}
	return time.Month(month), day, true
}

// InSeason reports whether now falls in the holiday's window.
func (h SeasonalCustomHoliday) InSeason(now time.Time) bool {
	return inYearlyWindow(now, h.Start, h.End)
}

func (seasonalRecipe) Definition() RecipeDefinition {
	return RecipeDefinition{
		Type:             "seasonal_themed",
		Category:         CategorySeasonal,
		SupportsRotation: false,
		AvoidDuplicates:  false,
		Presets: []GalleryPreset{
			{
				Key:              "se_auto",
				DisplayName:      "Seasonal Picks",
				Icon:             "🗓️",
				DescriptionShort: "Cycles automatically — Halloween in October, Christmas in December, etc.",
				DescriptionLong:  "One section that swaps in curated holiday picks based on the calendar. Choose the holidays, a vibe for each, and add your own holidays.",
				DefaultParams:    json.RawMessage(DefaultSeasonalConfig),
			},
			{
				Key:              "se_family_movie_night",
				DisplayName:      "Family Movie Night",
				Icon:             "🍿",
				DescriptionShort: "Family picks on Friday and Saturday evenings.",
				DefaultParams:    json.RawMessage(`{"enabled_themes":["family_movie_night"]}`),
			},
		},
	}
}

// DefaultSeasonalConfig is the config the Seasonal Picks preset and the
// built-in home row start from.
const DefaultSeasonalConfig = `{"enabled_themes":["halloween","christmas","new_year","valentines","st_patricks","easter","thanksgiving","summer_blockbuster","saturday_morning"]}`

// SeasonalPick is what a seasonal row shows right now: a theme (built-in or
// custom), the list behind it, the row's in-season title, and how the web home
// page dresses the row for it.
type SeasonalPick struct {
	// Theme is the built-in theme key; empty for a custom holiday.
	Theme      string
	Vibe       string
	ListURL    string
	Title      string
	Appearance SeasonalAppearance
}

// ActiveSeasonalPick returns what the row shows at now, or false when nothing
// is in season. Custom holidays come first, in the order saved, then the
// enabled built-in themes in SeasonalThemeOrder, else a legacy single theme
// (which a row pinned with mode "pinned" always shows).
func ActiveSeasonalPick(p SeasonalThemedParams, now time.Time) (SeasonalPick, bool) {
	for _, h := range p.CustomHolidays {
		if h.InSeason(now) {
			title := strings.TrimSpace(h.Title)
			if title == "" {
				title = strings.TrimSpace(h.Name)
			}
			return SeasonalPick{ListURL: h.ListURL, Title: title, Appearance: usableAppearance(h.Appearance)}, true
		}
	}
	var theme string
	switch {
	case len(p.EnabledThemes) > 0:
		theme = p.ActiveTheme(now)
	case p.Theme != "":
		_, known := SeasonalPredicates[p.Theme]
		if known && (p.Mode == "pinned" || p.themeInSeason(p.Theme, now)) {
			theme = p.Theme
		}
	}
	entry, ok := SeasonalThemeByKey(theme)
	if !ok {
		return SeasonalPick{}, false
	}
	vibe := entry.Vibe(p.Vibes[theme])
	title := strings.TrimSpace(p.ThemeTitles[theme])
	if title == "" {
		title = vibe.Title
	}
	return SeasonalPick{Theme: theme, Vibe: vibe.Key, ListURL: vibe.ListURL, Title: title, Appearance: usableAppearance(p.ThemeAppearance[theme])}, true
}

// usableAppearance trims an appearance and drops a background reference the
// server did not write, so a hand-edited config can never name a storage key.
// The text is left as saved; only the reference is a security-relevant value.
func usableAppearance(a SeasonalAppearance) SeasonalAppearance {
	a = a.Trimmed()
	if a.Background != "" && !SeasonalBackgroundRefPattern.MatchString(a.Background) {
		a.Background = ""
	}
	if a.Empty() {
		return SeasonalAppearance{}
	}
	return a
}

// SeasonalListURLsAround returns the list URLs a row will show from now
// through now+lookahead, checked hourly so hour-based themes count. The
// refresher fetches these ahead of time so a row has its list the moment its
// window opens.
func SeasonalListURLsAround(p SeasonalThemedParams, now time.Time, lookahead time.Duration) []string {
	seen := map[string]bool{}
	var urls []string
	for at := now; !at.After(now.Add(lookahead)); at = at.Add(time.Hour) {
		pick, ok := ActiveSeasonalPick(p, at)
		if ok && !seen[pick.ListURL] {
			seen[pick.ListURL] = true
			urls = append(urls, pick.ListURL)
		}
	}
	return urls
}

// ActiveTheme returns the highest-priority enabled theme in season at now,
// honoring the row's ThemeDates. Returns "" when no enabled theme is in
// season. Themes not in SeasonalThemeOrder are skipped silently.
func (p SeasonalThemedParams) ActiveTheme(now time.Time) string {
	if len(p.EnabledThemes) == 0 {
		return ""
	}
	enabledSet := make(map[string]bool, len(p.EnabledThemes))
	for _, t := range p.EnabledThemes {
		enabledSet[t] = true
	}
	for _, t := range SeasonalThemeOrder {
		if enabledSet[t] && p.themeInSeason(t, now) {
			return t
		}
	}
	return ""
}

func init() {
	Register(seasonalRecipe{})
}
