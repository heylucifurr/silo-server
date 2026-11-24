# Seasonal rows (WeFlix)

The Seasonal Picks home row shows titles for whichever holiday is in season. Upstream Silo owns
the row itself: holidays, their windows, the curated MDBList lists, and the order holidays are
picked in. WeFlix adds an **appearance** per holiday that turns the row into a banner on the web
home page, and a precedence rule that keeps a dressed holiday on screen when its window overlaps
another. The mechanics are in [seasonal-picks.md](../architecture/seasonal-picks.md); this page is
what WeFlix adds and what to check when upstream changes.

## What an admin sets

Admin → Home → Sections → the Seasonal Picks row (source "Changes with the season") → Edit. Each
holiday has:

| Control | Stored in the row's config | Owner |
|---|---|---|
| On/off checkbox | `enabled_themes` | upstream |
| Preset ("Christmas favorites") | `vibes[theme]` | upstream |
| Name in season | `theme_titles[theme]` | upstream |
| In season MM-DD to MM-DD | `theme_dates[theme]` (`{start, end}`; empty = the holiday's default dates; a window may wrap the year end) | upstream |
| Appearance: background, headline, subtitle, button label | `theme_appearance[theme]` (`{background, headline, subtitle, button_label}`); a custom holiday carries `custom_holidays[].appearance` | WeFlix |

The config is the section's JSONB `config` column, so no migration backs any of it. The
background is a content reference (`<16 hex>.webp`) returned by the upload, never a URL; any other
value is dropped when read. Backgrounds are stored in the artwork blob store under
`seasonal-backgrounds/`.

## Which holiday the row shows

`recipes.ActiveSeasonalPick`, evaluated at request time:

1. The first custom holiday in season.
2. Otherwise, among the enabled built-in holidays in season, **the first one with an appearance**
   in `SeasonalThemeOrder`; if none has one, the first in that order (upstream's rule).
3. Otherwise a legacy single-theme row.

A holiday stays in season 12 hours past its last day. Nothing in season, or a holiday whose list
matches nothing the viewer can see, gives an empty row, and the Home API leaves empty rows out.

Rule 2 is WeFlix's. Upstream ranks Thanksgiving (Nov 22–30) above Christmas, so a Christmas window
moved to start in early November gave way to Thanksgiving for the overlap and the banner
disappeared until December. A holiday the admin dressed is one they want seen, so it wins the
overlap; rows with no appearances behave exactly as upstream's.

## API

- `GET /api/v2/home/layout` (and library layouts): a seasonal entry carries an optional
  `appearance` object (`background_url`, `headline`, `subtitle`, `button_label`) while a dressed
  holiday is in season; absent for every other row and off season. The row's `title` is the
  holiday's name in season.
- `GET /api/v2/home/sections/{id}/items`: the row's cards, from the in-season holiday's list.
- `POST /api/v2/admin/sections/seasonal-backgrounds`: admin upload (JPEG, PNG or WebP), re-encoded
  to WebP at most 2560 wide; answers the reference.
- `GET /api/v2/seasonal-backgrounds/{ref}`: the stored image, cacheable forever.
- The frozen `/api/v1` layout never carried `appearance`.

## Web

`Home.tsx` hands each row its layout entry's `appearance`. When `isSeasonalWebRow` (in
`lib/seasonalBand.ts`) says the row is a band, `SeasonalHeroRow` draws it: background on the
left with headline, subtitle and a button that opens the row's own catalog page (empty label =
"Watch now"), poster cards to the right at the Featured size. Without an appearance the row is an
ordinary `SectionRow`.

## Apps

The iOS, tvOS and Android apps read the same row and ignore `appearance`, so they show it as a
normal row with the in-season holiday's titles. Rule 2 changes which holiday's titles they show
during an overlap, in step with the web.

## Files that are WeFlix's

Server:

- `internal/sections/recipes/seasonal_weflix.go` and `seasonal_weflix_test.go`: holiday precedence
  (rule 2) and its tests.
- `internal/sections/recipes/seasonal.go`: `SeasonalAppearance`, `ThemeAppearance`,
  `usableAppearance`, the `Appearance` on `SeasonalPick`, and the one-line hook
  `theme = p.weflixActiveTheme(now)` in `ActiveSeasonalPick`.
- `internal/api/handlers/section_appearance.go`: `seasonalAppearanceOf`.
- `internal/api/handlers/library_views_service.go`: `Appearance: seasonalAppearanceOf(s, now)` in
  `sectionLayoutOf`.
- `internal/api/handlers/sections.go`: `Appearance` on `resolvedSectionLayoutResponse`
  (`json:"-"`).
- `internal/apiv2/library_views.go`: `SectionAppearance`, the `appearance` member and its copy in
  `sectionLayoutOf`.
- `internal/api/handlers/seasonal_backgrounds.go`, `internal/apiv2/seasonal_backgrounds.go`,
  `internal/seasonalbackground/`, the routes in `internal/api/router.go` and
  `internal/apiv2/router.go`.
- Tests: `internal/apiv2/section_layout_appearance_test.go`,
  `internal/seasonalbackground/service_test.go`, the appearance cases in
  `internal/sections/recipes/seasonal_test.go`.

Web:

- `web/src/components/SeasonalHeroRow.tsx` (+ test), `web/src/lib/seasonalBand.ts` (+ test).
- `web/src/pages/Home.tsx`: the `appearance` prop and the `isSeasonalWebRow` branch in
  `DeferredHomeSection`.
- `web/src/components/SectionRow.tsx` and `web/src/components/MediaCarousel.tsx`: the `lead` slide
  and band card sizing.
- `web/src/components/homeRows/addRow/SeasonalAppearanceFields.tsx`, the appearance block in
  `SeasonalFields.tsx`, `web/src/lib/homeRows/seasonal.ts`,
  `web/src/hooks/queries/seasonalBackground.ts`, the band widths in `web/src/lib/uiCustomization.ts`.

## When merging upstream, check these

- `ActiveSeasonalPick` still calls `p.weflixActiveTheme(now)` (not `p.ActiveTheme(now)`), and
  `SeasonalPick` still carries `Appearance`.
- `sectionLayoutOf` in both `internal/api/handlers/library_views_service.go` and
  `internal/apiv2/library_views.go` still copies the appearance; the v2 Home layout
  (`internal/apiv2/home.go`) still builds its entries through `sectionLayoutOf`.
- `SeasonalThemedParams` still has `theme_appearance` and `custom_holidays[].appearance`, and
  nothing parses section configs strictly (unknown keys rejected or stripped).
- The fetcher still wires `SeasonalLists`, `SeasonalResolver` and `SeasonalNeighbors`
  (`internal/api/router.go`, `internal/sections/fetcher.go`).
- `Home.tsx` still passes `appearance` to the row and branches on `isSeasonalWebRow`.
- Regenerate `contracts/api/v2/openapi.json` and `web/src/api/v2/schema.ts` and confirm
  `SectionAppearance` and the background routes are still in them.
- Run `go test ./internal/sections/recipes/ -run 'Seasonal|Weflix'`,
  `./internal/apiv2/ -run Appearance`, and
  `cd web && pnpm exec vitest run src/lib/seasonalBand src/components/SeasonalHeroRow`.
- A row that disappears in season is first a date question: check which holidays are in season
  today with the row's `theme_dates` (overlaps follow rule 2), and that the in-season holiday's
  list matches titles in the library.
