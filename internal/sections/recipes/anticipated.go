package recipes

import (
	"encoding/json"
	"time"
)

// AnticipatedParams configures the anticipated resolver. The curated list is
// server-wide and admin-owned, so a row of this type carries no parameters in
// v1; the struct exists so a later list selection can be added without a
// contract change.
type AnticipatedParams struct{}

type anticipatedRecipe struct{}

func (anticipatedRecipe) Type() string                   { return "anticipated" }
func (anticipatedRecipe) NewParams() any                 { return &AnticipatedParams{} }
func (anticipatedRecipe) DefaultCacheTTL() time.Duration { return 15 * time.Minute }

func (anticipatedRecipe) Resolve(rc ResolverContext) (ResolvedItems, error) {
	return delegateResolve("anticipated", rc)
}

func (anticipatedRecipe) Validate(raw json.RawMessage) error { return nil }

func (anticipatedRecipe) Definition() RecipeDefinition {
	return RecipeDefinition{
		Type:            "anticipated",
		Category:        CategoryDiscovery,
		AvoidDuplicates: true,
		Presets: []GalleryPreset{
			{
				Key:              "anticipated_default",
				DisplayName:      "Most Anticipated",
				Icon:             "🎬",
				DescriptionShort: "Upcoming titles you do not have yet.",
				DefaultParams:    json.RawMessage(`{}`),
			},
		},
	}
}

func init() {
	Register(anticipatedRecipe{})
}
