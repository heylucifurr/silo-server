package heroclips

import (
	"context"
	"time"

	"github.com/Silo-Server/silo-server/internal/artworkurl"
)

// ClipPathSource reads which items have a published clip, and where it lives.
type ClipPathSource interface {
	ClipPathsFor(ctx context.Context, contentIDs []string) (map[string]string, error)
}

// AnticipatedClipPathSource reads which curated titles have a published clip in
// anticipated_clips, keyed by the slide's synthetic id.
type AnticipatedClipPathSource interface {
	AnticipatedClipPathsFor(ctx context.Context, contentIDs []string) (map[string]string, error)
}

// ClipURLTTL is how long a clip's URL has to stay good. A hero clip plays for
// minutes and a media element re-requests as it plays, so the URL outlives the
// artwork lifetime a poster is served with.
const ClipURLTTL = 12 * time.Hour

// URLResolver turns published clips into URLs a client can fetch, through the
// same signing the item's artwork goes through.
type URLResolver struct {
	Clips ClipPathSource
	// Anticipated reads clips for the curated titles the library does not own,
	// keyed by their synthetic id. Nil leaves those slides without a clip, which
	// is the ordinary case.
	Anticipated AnticipatedClipPathSource
	Resolver    artworkurl.Resolver
	TTL         time.Duration
}

// HeroVideoURLs returns the URL of each item's clip, keyed by content id, for
// the items that have one. An item without a clip is simply absent, which is
// how the hero knows to keep its backdrop.
//
// A failure is the caller's to log and ignore: a hero without a clip is the
// ordinary case, not an error.
func (r URLResolver) HeroVideoURLs(ctx context.Context, contentIDs []string) (map[string]string, error) {
	if r.Clips == nil {
		return nil, nil
	}
	return r.sign(ctx, contentIDs, func() (map[string]string, error) {
		return r.Clips.ClipPathsFor(ctx, contentIDs)
	})
}

// AnticipatedHeroVideoURLs is HeroVideoURLs for an unowned anticipated slide,
// whose clip lives in anticipated_clips rather than hero_clips. The ids are the
// slide's synthetic ids, so the URL comes back under the key the card uses.
func (r URLResolver) AnticipatedHeroVideoURLs(ctx context.Context, contentIDs []string) (map[string]string, error) {
	if r.Anticipated == nil {
		return nil, nil
	}
	return r.sign(ctx, contentIDs, func() (map[string]string, error) {
		return r.Anticipated.AnticipatedClipPathsFor(ctx, contentIDs)
	})
}

// sign turns the paths find returns into signed URLs, dropping any path that
// cannot be signed.
func (r URLResolver) sign(ctx context.Context, contentIDs []string, find func() (map[string]string, error)) (map[string]string, error) {
	if r.Resolver == nil || len(contentIDs) == 0 {
		return nil, nil
	}

	paths, err := find()
	if err != nil {
		return nil, err
	}

	ttl := r.TTL
	if ttl <= 0 {
		ttl = ClipURLTTL
	}

	urls := make(map[string]string, len(paths))
	for contentID, path := range paths {
		resolved, ok := artworkurl.ResolveURLFor(ctx, r.Resolver, path, ttl)
		if !ok || resolved.URL == "" {
			// One unsignable path must not cost the row its other clips: the
			// item keeps its backdrop like any title without a clip.
			continue
		}
		urls[contentID] = resolved.URL
	}

	return urls, nil
}
