package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/heroclips"
	"github.com/Silo-Server/silo-server/internal/taskmanager"
)

// heroClipsPerRun bounds one run. A run that stops at the bound leaves the rest
// for the next one, so a large feature refresh cannot hold a task slot for hours.
const heroClipsPerRun = 12

// heroClipRetryAfter is how long a title that failed waits before another try.
// A trailer that 404s today is usually still gone tomorrow.
const heroClipRetryAfter = 24 * time.Hour

// HeroClipCandidates lists the titles a run should make clips for.
type HeroClipCandidates interface {
	ClipCandidates(ctx context.Context, recipe string, retryAfter time.Duration, limit int) ([]heroclips.Candidate, error)
}

// AnticipatedClipCandidates lists the curated titles a run should make clips for.
//
// It is deliberately a separate interface from HeroClipCandidates even though
// the two queries share a shape: *heroclips.Repository answers both, so pointing
// the curated pass at ClipCandidates would hand it the featured rows' catalog
// titles instead of the curated ones, and every one of them would be rejected as
// not an anticipated id. The distinct method name makes that wiring impossible.
type AnticipatedClipCandidates interface {
	AnticipatedCandidates(ctx context.Context, recipe string, retryAfter time.Duration, limit int) ([]heroclips.Candidate, error)
}

// HeroClipGenerator makes one title's clip. Satisfied by *heroclips.Generator.
type HeroClipGenerator interface {
	Create(ctx context.Context, candidate heroclips.Candidate) error
}

// GenerateHeroClipsTask gives the titles the featured rows hold a hero clip:
// a short cut of the title's own trailer, played muted behind the backdrop.
//
// It runs as a task rather than on a page load, so an administrator can see it,
// schedule it, or turn it off where every other piece of background work is
// managed.
type GenerateHeroClipsTask struct {
	candidates HeroClipCandidates
	generator  HeroClipGenerator
	recipe     heroclips.Recipe
	// anticipatedCandidates and anticipatedGenerator run a second pass over the
	// curated "Most Anticipated" titles the library does not own. Nil skips it.
	anticipatedCandidates AnticipatedClipCandidates
	anticipatedGenerator  HeroClipGenerator
}

// WithAnticipated adds the pass over the curated titles the library does not own,
// whose clips live in anticipated_clips rather than hero_clips. Returns the task
// so wiring reads as one expression.
func (t *GenerateHeroClipsTask) WithAnticipated(candidates AnticipatedClipCandidates, generator HeroClipGenerator) *GenerateHeroClipsTask {
	t.anticipatedCandidates = candidates
	t.anticipatedGenerator = generator
	return t
}

func NewGenerateHeroClipsTask(candidates HeroClipCandidates, generator HeroClipGenerator, recipe heroclips.Recipe) *GenerateHeroClipsTask {
	return &GenerateHeroClipsTask{candidates: candidates, generator: generator, recipe: recipe}
}

func (t *GenerateHeroClipsTask) Key() string  { return "generate_hero_clips" }
func (t *GenerateHeroClipsTask) Name() string { return "Generate Hero Clips" }
func (t *GenerateHeroClipsTask) Description() string {
	return "Downloads the trailers of the titles the featured rows hold and cuts each into the short clip a video hero plays behind its backdrop."
}
func (t *GenerateHeroClipsTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryMetadata
}
func (t *GenerateHeroClipsTask) IsHidden() bool { return false }
func (t *GenerateHeroClipsTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeStartup},
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: int64((24 * time.Hour) / time.Millisecond)},
	}
}

func (t *GenerateHeroClipsTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	ownedWired := t.candidates != nil && t.generator != nil
	anticipatedWired := t.anticipatedCandidates != nil && t.anticipatedGenerator != nil
	if !ownedWired && !anticipatedWired {
		progress.Report(100, "Hero clips are not configured")
		return nil
	}

	candidates := []heroclips.Candidate(nil)
	if ownedWired {
		var err error
		candidates, err = t.candidates.ClipCandidates(ctx, t.recipe.String(), heroClipRetryAfter, heroClipsPerRun)
		if err != nil {
			return fmt.Errorf("list hero clip candidates: %w", err)
		}
	}

	// Create records its own failures on the title and reports success, so the
	// run keeps going and the row is what makes a broken trailer wait.
	for index, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		progress.Report(float64(index)*100/float64(len(candidates)), fmt.Sprintf("Making a clip for %s", candidate.ContentID))
		if err := t.generator.Create(ctx, candidate); err != nil {
			return fmt.Errorf("generate hero clip for %s: %w", candidate.ContentID, err)
		}
	}

	made := len(candidates)
	// The curated pass runs whether or not the featured rows had work: a hero row
	// of curated titles the library owns none of is the ordinary case, and a run
	// that stopped at an empty catalog pass would never make those clips.
	if anticipatedWired {
		anticipated, err := t.anticipatedCandidates.AnticipatedCandidates(ctx, t.recipe.String(), heroClipRetryAfter, heroClipsPerRun)
		if err != nil {
			return fmt.Errorf("list anticipated clip candidates: %w", err)
		}
		for index, candidate := range anticipated {
			if err := ctx.Err(); err != nil {
				return err
			}
			progress.Report(float64(index)*100/float64(len(anticipated)), fmt.Sprintf("Making a clip for %s", candidate.ContentID))
			// The generator records its own failure and reports success, so one
			// unreachable trailer does not stop the pass.
			if err := t.anticipatedGenerator.Create(ctx, candidate); err != nil {
				return fmt.Errorf("generate anticipated clip for %s: %w", candidate.ContentID, err)
			}
		}
		made += len(anticipated)
	}

	progress.Report(100, fmt.Sprintf("Checked %d featured titles", made))

	return nil
}
