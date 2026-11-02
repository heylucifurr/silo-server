-- +goose Up
-- +goose StatementBegin
-- A movie or series an administrator curated as "Most Anticipated": an upcoming
-- title the library does not own yet, featured on Home as the Coming Soon hero.
-- The list is server-wide and admin-owned. No column stores or is named like a
-- catalog content ID: re-anchoring, merging and deleting catalog items never
-- reach these tables, and a title that arrives in the library is matched at read
-- time by its provider IDs. The id is minted by idgen and never exposed.
CREATE TABLE public.anticipated_titles (
    id bigint PRIMARY KEY,
    media_type text NOT NULL CHECK (media_type IN ('movie', 'series')),
    tmdb_id integer NOT NULL CHECK (tmdb_id > 0),
    imdb_id text NOT NULL DEFAULT '',
    tvdb_id integer,
    -- The administrator's drag order on the curation page.
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    title text NOT NULL DEFAULT '',
    original_title text NOT NULL DEFAULT '',
    year integer,
    release_date date,
    overview text NOT NULL DEFAULT '',
    -- TMDB path fragments, not asset-store keys: an unowned title's artwork is
    -- served straight from TMDB's image CDN.
    poster_path text NOT NULL DEFAULT '',
    backdrop_path text NOT NULL DEFAULT '',
    logo_path text NOT NULL DEFAULT '',
    vote_average numeric(3,1),
    runtime integer,
    episode_runtime integer,
    -- US certification, the one the requests rating ceiling filters on.
    certification text NOT NULL DEFAULT '',
    -- The title's own trailer, read from TMDB videos; the source of its hero clip.
    trailer_site text NOT NULL DEFAULT '',
    trailer_key text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'needs_review', 'removed')),
    -- Consecutive TMDB 404s; two at least a day apart confirm a deletion.
    not_found_count integer NOT NULL DEFAULT 0,
    last_not_found_at timestamptz,
    last_checked_at timestamptz,
    next_check_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (media_type, tmdb_id)
);
CREATE INDEX idx_anticipated_titles_order ON public.anticipated_titles (position, id);

-- Every provider ID a title has held, so a repointed TMDB ID still finds the
-- entry and a duplicate is recognized. Mirrors watchlist_title_aliases.
CREATE TABLE public.anticipated_aliases (
    title_id bigint NOT NULL REFERENCES public.anticipated_titles(id) ON DELETE CASCADE,
    media_type text NOT NULL,
    provider text NOT NULL CHECK (provider IN ('tmdb', 'imdb', 'tvdb')),
    provider_id text NOT NULL,
    PRIMARY KEY (media_type, provider, provider_id)
);
CREATE INDEX idx_anticipated_aliases_title ON public.anticipated_aliases (title_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS public.anticipated_aliases;
DROP TABLE IF EXISTS public.anticipated_titles;
-- +goose StatementEnd
