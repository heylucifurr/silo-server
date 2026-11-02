# Anticipated (the curated Most Anticipated row)

An administrator hand-picks upcoming movies and series the library does not own
yet. The list is served as the `anticipated` Home row, which can be marked
featured to become the Coming Soon hero. Each entry carries the title's own
lifecycle: it reads as Coming soon while the library lacks it, and the same
entry plays once a copy arrives.

## The store

`anticipated_titles` holds one row per curated title and `anticipated_aliases`
holds every provider ID a title has held. The list is server-wide and
admin-owned. Migration `20261102010837_anticipated_titles.sql`.

No column stores or is named like a catalog content ID. This is the invariant the
external-watchlist tables keep too (`docs/architecture/external-watchlist.md`):
catalog re-anchoring, merging and deleting never reach these tables. A title that
arrives in the library is matched at read time by its provider IDs.

An entry carries a display snapshot read from TMDB (title, year, release date,
overview, poster/backdrop/logo paths, vote average, runtime, US certification)
and the title's own trailer (`trailer_site`, `trailer_key`). Because an unowned
title has no `item_videos` row, TMDB's videos endpoint is the only trailer source
(`metadata/tmdb.Client.Videos`).

`position` is the administrator's order. `ReplaceAll` writes the whole list in one
transaction: it drops entries the admin removed, keeps the snapshot of an entry
that stays (matched on `media_type` + `tmdb_id`), renumbers positions from the
input order, and re-records aliases.

## Curating it

`internal/anticipated.Service` is the curation surface. `Replace` resolves any
entry with no snapshot yet through TMDB, then stores the list. `Refresh` re-reads
the entries that are due: a failed read keeps the last good snapshot, records the
failure, and moves the entry toward `needs_review` then `removed` after repeated
misses — an entry is never deleted by the server. `internal/taskmanager/tasks`
runs that refresh at startup and every six hours.

The admin API (`/api/v2/admin/anticipated`) reads the list, replaces it with an
`If-Match` guard over a content ETag, searches TMDB for titles to add, and forces
a refresh. The `/search` results carry raw TMDB `poster_path`/`backdrop_path`
fragments, exactly as `RequestMediaResult` does.

## The row

`anticipated` is a recipe (`internal/sections/recipes/anticipated.go`), so it
appears in `listSectionRecipes` and therefore in the admin's Add-a-row gallery
with no extra registration. `SectionAnticipated` is in `ValidSectionTypes`.

`fetchAnticipated` reads the list in admin order and resolves each entry:

- A title the library owns becomes its ordinary catalog item, so the slide plays,
  is filtered by the viewer's access, and uses the existing `hero_clips` path.
- A title the library lacks becomes a **synthetic external item** whose
  `content_id` is `ext:<media_type>-<tmdb_id>` (`sections.ExternalContentIDPrefix`).
  Its `Title`, `Overview`, `Year`, `ReleaseDate` and rating come from the
  snapshot, and its artwork paths are TMDB fragments.

Matching uses `catalog.GetByExternalIDs`, one batched lookup per media type, the
same call the trending refresher and seasonal lists use.

The row is row-backed, so the catalog's Explore-all pages it past its item limit
for free.

### How a slide reads

`sections.AnticipatedAvailability(item, now)` decides, at read time and with no
stored state:

- a real catalog item is `in_library`, and the slide offers Play;
- a synthetic item is `unreleased` while its release date is in the future, else
  `released_unowned`.

Nothing writes on promotion — the read decides — so a node dying mid-anything
cannot corrupt the lifecycle, and no race exists between nodes.

## The card on the wire

`/api/v2` section cards gained additive members, present only on an unowned
anticipated slide: `availability`, `tmdb_id`, `in_watchlist`, `requestable`.
`poster_url`, `backdrop_url` and `logo_url` on such a slide are composed from
TMDB's image CDN (`sections.TMDBImageURL`), not from the asset store. Every other
card is unchanged.

`getHomeCapabilities` reports `anticipated_supported` so a client offers the row
only when the server serves it.

The row is a discovery row, so it honours a profile's Not interested marks
(`HidesNotInterested`), which matters for entries the library owns.

## Not built yet

Trailer motion for an unowned slide. `anticipated_clips`
(`20261102012437_anticipated_clips.sql`) and its migration exist, and the table
follows `hero_clips` (same recipe, same `hero-clips/` object namespace), but the
candidate query, publisher, task wiring, `hero_video_url` delivery and `blobgc`
liveness are not implemented, so an unowned slide keeps its backdrop. The web
client renders the hero slide and the admin curation page are likewise pending.
