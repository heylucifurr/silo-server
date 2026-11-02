-- +goose Up
-- +goose StatementBegin
-- A curated "Most Anticipated" title the library does not own has no media_items
-- row, so its hero clip cannot live in hero_clips — that table cascades from
-- media_items. This is the twin for those titles: one clip per curated entry,
-- keyed by media type and TMDB id, sharing hero_clips' recipe and the same
-- hero-clips/ object namespace. Liveness comes from the entry itself, not from a
-- catalog item, so the sweep reads this table as a second liveness source.
--
-- The object key is hero-clips/<hash of the synthetic item id>/<recipe>.mp4,
-- which the existing blob_gc_queue prefix constraint already admits, so this
-- migration adds no new prefix pattern.
CREATE TABLE IF NOT EXISTS public.anticipated_clips (
    media_type  text        NOT NULL CHECK (media_type IN ('movie', 'series')),
    tmdb_id     integer     NOT NULL CHECK (tmdb_id > 0),
    clip_path   text        NOT NULL DEFAULT '',
    recipe      text        NOT NULL DEFAULT '',
    source      text        NOT NULL DEFAULT '',
    source_key  text        NOT NULL DEFAULT '',
    duration_ms integer     NOT NULL DEFAULT 0,
    width       integer     NOT NULL DEFAULT 0,
    height      integer     NOT NULL DEFAULT 0,
    bytes       bigint      NOT NULL DEFAULT 0,
    state       text        NOT NULL DEFAULT 'ready' CHECK (state IN ('ready', 'failed')),
    error       text        NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (media_type, tmdb_id)
);

-- A failed clip is retried after a pause, so the generator reads by state.
CREATE INDEX IF NOT EXISTS idx_anticipated_clips_state ON public.anticipated_clips (state, updated_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS public.idx_anticipated_clips_state;
DROP TABLE IF EXISTS public.anticipated_clips;
-- +goose StatementEnd
