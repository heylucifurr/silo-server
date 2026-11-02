package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/heroclips"
)

type stubClipCandidates struct {
	items []heroclips.Candidate
	err   error
	calls int
}

func (s *stubClipCandidates) ClipCandidates(context.Context, string, time.Duration, int) ([]heroclips.Candidate, error) {
	s.calls++
	return s.items, s.err
}

// stubAnticipatedCandidates stands in for the curated pass's source. It answers
// AnticipatedCandidates, not ClipCandidates, so a test cannot accidentally wire
// the curated pass to the featured rows' query.
type stubAnticipatedCandidates struct {
	items []heroclips.Candidate
	err   error
}

func (s *stubAnticipatedCandidates) AnticipatedCandidates(context.Context, string, time.Duration, int) ([]heroclips.Candidate, error) {
	return s.items, s.err
}

type stubClipGenerator struct {
	made []string
	err  error
}

func (s *stubClipGenerator) Create(_ context.Context, c heroclips.Candidate) error {
	s.made = append(s.made, c.ContentID)
	return s.err
}

func TestGenerateHeroClipsIncludesAnticipated(t *testing.T) {
	owned := &stubClipCandidates{items: []heroclips.Candidate{{ContentID: "tvdb-372264"}}}
	ownedGen := &stubClipGenerator{}
	anticipated := &stubAnticipatedCandidates{items: []heroclips.Candidate{
		{ContentID: heroclips.AnticipatedCandidateID("movie", 1083381)},
	}}
	anticipatedGen := &stubClipGenerator{}

	task := NewGenerateHeroClipsTask(owned, ownedGen, heroclips.NewRecipe(0)).
		WithAnticipated(anticipated, anticipatedGen)
	if err := task.Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(ownedGen.made) != 1 || len(anticipatedGen.made) != 1 {
		t.Fatalf("made = %v / %v, want one clip each", ownedGen.made, anticipatedGen.made)
	}

	// The catalog pass runs first, so a failure listing the curated titles still
	// leaves the featured rows' clips made.
	ownedGen.made = nil
	broken := &stubAnticipatedCandidates{err: errors.New("boom")}
	task = NewGenerateHeroClipsTask(owned, ownedGen, heroclips.NewRecipe(0)).
		WithAnticipated(broken, &stubClipGenerator{})
	if err := task.Execute(context.Background(), noopProgress{}); err == nil {
		t.Fatal("Execute: want an error when the anticipated listing fails")
	}
	if len(ownedGen.made) != 1 {
		t.Fatalf("the catalog pass did not finish first: %v", ownedGen.made)
	}

	// Without the second pass the task behaves exactly as it did before.
	only := &stubClipGenerator{}
	if err := NewGenerateHeroClipsTask(owned, only, heroclips.NewRecipe(0)).
		Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute without anticipated: %v", err)
	}
	if len(only.made) != 1 {
		t.Fatalf("made = %v, want the catalog candidate only", only.made)
	}
}

// A hero row of curated titles the library owns none of is the ordinary case for
// Most Anticipated: the featured rows have nothing to clip, and the curated pass
// still has to run. An empty catalog pass used to end the run before reaching it.
func TestGenerateHeroClipsRunsAnticipatedWhenFeaturedRowsAreEmpty(t *testing.T) {
	owned := &stubClipCandidates{}
	anticipated := &stubAnticipatedCandidates{items: []heroclips.Candidate{
		{ContentID: heroclips.AnticipatedCandidateID("movie", 1003596)},
	}}
	anticipatedGen := &stubClipGenerator{}

	task := NewGenerateHeroClipsTask(owned, &stubClipGenerator{}, heroclips.NewRecipe(0)).
		WithAnticipated(anticipated, anticipatedGen)
	if err := task.Execute(context.Background(), noopProgress{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(anticipatedGen.made) != 1 {
		t.Fatalf("anticipated clips made = %v, want the curated candidate", anticipatedGen.made)
	}
}
