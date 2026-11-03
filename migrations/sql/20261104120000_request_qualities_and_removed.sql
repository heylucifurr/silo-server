-- +goose Up
-- +goose StatementBegin
-- Per-request quality choice. Targets already record what was fulfilled per
-- quality; this column records the intent, so a pending request knows what it
-- asks for and two requests for one title can coexist when their qualities do
-- not overlap. Existing rows default to HD, then submitted requests are
-- backfilled from the qualities they were actually sent for.
ALTER TABLE public.media_requests
    ADD COLUMN IF NOT EXISTS requested_qualities text[] NOT NULL DEFAULT '{1080p}';

UPDATE public.media_requests mr
SET requested_qualities = sub.qualities
FROM (
    SELECT request_id, array_agg(DISTINCT quality ORDER BY quality) AS qualities
    FROM public.media_request_targets
    GROUP BY request_id
) sub
WHERE sub.request_id = mr.id;

ALTER TABLE public.media_requests
    ADD CONSTRAINT media_requests_requested_qualities_check
        CHECK (
            array_length(requested_qualities, 1) >= 1
            AND requested_qualities <@ ARRAY['1080p', '2160p']::text[]
        );

-- A request deleted from the queue is soft-removed: the row and its history
-- stay, so reconciliation and the audit trail keep working.
ALTER TABLE public.media_requests
    DROP CONSTRAINT IF EXISTS media_requests_outcome_check;
ALTER TABLE public.media_requests
    ADD CONSTRAINT media_requests_outcome_check
        CHECK (outcome IN ('active', 'declined', 'cancelled', 'failed', 'removed'));

-- One title may hold an HD request and a 4K request at once. Overlapping
-- qualities are refused in code under a per-title advisory lock, so the old
-- one-active-row-per-title index no longer applies.
DROP INDEX IF EXISTS public.idx_media_requests_active_tmdb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_requests_active_tmdb
    ON public.media_requests (media_type, provider, tmdb_id)
    WHERE outcome = 'active' AND status <> 'completed';

ALTER TABLE public.media_requests
    DROP CONSTRAINT IF EXISTS media_requests_outcome_check;
ALTER TABLE public.media_requests
    ADD CONSTRAINT media_requests_outcome_check
        CHECK (outcome IN ('active', 'declined', 'cancelled', 'failed'));

ALTER TABLE public.media_requests
    DROP CONSTRAINT IF EXISTS media_requests_requested_qualities_check;
ALTER TABLE public.media_requests
    DROP COLUMN IF EXISTS requested_qualities;
-- +goose StatementEnd
