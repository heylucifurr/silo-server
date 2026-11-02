package apiv2

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/Silo-Server/silo-server/internal/anticipated"
)

// Anticipated administration: the admin-curated "Most Anticipated" list behind
// the anticipated Home row.

const (
	opGetAdminAnticipated     = "getAdminAnticipated"
	opUpdateAdminAnticipated  = "updateAdminAnticipated"
	opSearchAdminAnticipated  = "searchAdminAnticipated"
	opRefreshAdminAnticipated = "refreshAdminAnticipated"
)

// anticipatedService is the curation surface this file reads. Satisfied by
// *anticipated.Service.
type anticipatedService interface {
	List(ctx context.Context) ([]anticipated.Title, error)
	Replace(ctx context.Context, in []anticipated.Input) ([]anticipated.Title, error)
	Search(ctx context.Context, mediaType, query string) ([]anticipated.Result, error)
	Refresh(ctx context.Context, limit int) (int, error)
}

// AdminAnticipatedTitle is one curated entry. poster_path and backdrop_path are
// raw TMDB fragments, exactly as RequestMediaResult carries them; the client
// composes the full URL.
type AdminAnticipatedTitle struct {
	MediaType    string   `json:"media_type" enum:"movie,series" example:"movie"`
	TMDBID       int      `json:"tmdb_id" example:"1083381"`
	Position     int      `json:"position" doc:"The admin's order on the hero" example:"0"`
	Title        string   `json:"title" example:"Avengers: Doomsday"`
	Year         int      `json:"year,omitempty" example:"2026"`
	ReleaseDate  string   `json:"release_date,omitempty" doc:"ISO date" example:"2026-12-18"`
	Overview     string   `json:"overview,omitempty"`
	PosterPath   string   `json:"poster_path,omitempty" doc:"TMDB image path fragment"`
	BackdropPath string   `json:"backdrop_path,omitempty" doc:"TMDB image path fragment"`
	TrailerKey   string   `json:"trailer_key,omitempty" doc:"The YouTube key the hero clip is cut from"`
	State        string   `json:"state" enum:"active,needs_review,removed"`
	RefreshedAt  *Instant `json:"refreshed_at,omitempty" doc:"When the entry's metadata was last read from TMDB"`
}

// AdminAnticipatedEntryBody is one entry an admin sends in a whole-list replace.
type AdminAnticipatedEntryBody struct {
	MediaType string `json:"media_type" enum:"movie,series" example:"movie"`
	TMDBID    int    `json:"tmdb_id" minimum:"1" example:"1083381"`
}

type AdminAnticipatedList struct {
	Entries []AdminAnticipatedTitle `json:"entries" doc:"In the admin's order. Empty, never null"`
	Max     int                     `json:"max" doc:"The most entries the list holds" example:"50"`
}

type AdminAnticipatedOutput struct {
	ETag string `header:"ETag" doc:"The list's current tag; send it as If-Match when replacing"`
	Body AdminAnticipatedList
}

type AdminAnticipatedUpdateInput struct {
	IfMatch     string `header:"If-Match" doc:"The ETag of the list being replaced. Omitted skips the check"`
	IfNoneMatch string `header:"If-None-Match"`
	Body        struct {
		Entries []AdminAnticipatedEntryBody `json:"entries" maxItems:"50" doc:"The whole list, in the order the hero should show it"`
	}
}

// AdminAnticipatedSearchInput finds titles to add.
type AdminAnticipatedSearchInput struct {
	MediaType string `query:"media_type" enum:"movie,series" default:"movie" example:"movie"`
	Query     string `query:"q" minLength:"1" maxLength:"120" doc:"Part of a title" example:"Avengers Doomsday"`
}

type AdminAnticipatedSearchResult struct {
	MediaType    string  `json:"media_type" enum:"movie,series" example:"movie"`
	TMDBID       int     `json:"tmdb_id" example:"1083381"`
	Title        string  `json:"title" example:"Avengers: Doomsday"`
	Year         int     `json:"year,omitempty" example:"2026"`
	ReleaseDate  string  `json:"release_date,omitempty" example:"2026-12-18"`
	PosterPath   string  `json:"poster_path,omitempty"`
	BackdropPath string  `json:"backdrop_path,omitempty"`
	Overview     string  `json:"overview,omitempty"`
	VoteAverage  float64 `json:"vote_average,omitempty" example:"8.2"`
}

type AdminAnticipatedSearchOutput struct{ Body Collection[AdminAnticipatedSearchResult] }

type AdminAnticipatedRefreshOutput struct {
	Body struct {
		Refreshed int `json:"refreshed" doc:"How many entries were re-read from TMDB"`
	}
}

func registerAdminAnticipated(reg *Registry) {
	op := func(method, path, id, summary string, guard bool) Operation {
		o := Operation{Operation: humaOp(method, Prefix+path, id, "admin-anticipated", summary),
			Class: ClassActingAdmin, DemoRestricted: isMutatingMethod(method), ServiceBacked: true, Guarded: guard}
		o.Errors = []int{http.StatusUnprocessableEntity}
		if method != http.MethodGet {
			o.RetrySafety = RetrySafetyNonRetryable
		}
		return o
	}
	Register(reg, op(http.MethodGet, "/admin/anticipated", opGetAdminAnticipated,
		"Read the curated Most Anticipated list.", false), reg.getAdminAnticipated)
	Register(reg, op(http.MethodPut, "/admin/anticipated", opUpdateAdminAnticipated,
		"Replace the curated Most Anticipated list.", true), reg.updateAdminAnticipated)
	search := op(http.MethodPost, "/admin/anticipated/search", opSearchAdminAnticipated,
		"Search TMDB for titles to add to the curated list.", false)
	search.RetrySafety = RetrySafetyNaturalIdempotent
	search.DemoRestricted = false
	Register(reg, search, reg.searchAdminAnticipated)
	Register(reg, op(http.MethodPost, "/admin/anticipated/refresh", opRefreshAdminAnticipated,
		"Re-read every entry that is due from TMDB.", false), reg.refreshAdminAnticipated)
}

func (reg *Registry) anticipatedService() (anticipatedService, *Problem) {
	s, ok := reg.deps.Anticipated.(anticipatedService)
	if !ok || s == nil {
		return nil, unavailable("anticipated curation")
	}
	return s, nil
}

// anticipatedETag tags the list by its content, so a replace that must not
// overwrite another admin's edit can send the tag it read.
func anticipatedETag(entries []anticipated.Title) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s:%d:%d:%s:%s|", e.MediaType, e.TMDBID, e.Position, e.Title, e.TrailerKey)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("\"%x\"", sum[:8])
}

// anticipatedProblem maps the curation service's own validation failure to a 422
// naming the admin's input; anything else is a service error.
func anticipatedProblem(err error) error {
	if err != nil && strings.Contains(err.Error(), "anticipated: ") {
		return NewProblem(TypeValidationFailed, err.Error())
	}
	return serviceProblem(err)
}

func adminAnticipatedTitles(entries []anticipated.Title) []AdminAnticipatedTitle {
	out := make([]AdminAnticipatedTitle, 0, len(entries))
	for _, e := range entries {
		row := AdminAnticipatedTitle{
			MediaType: e.MediaType, TMDBID: e.TMDBID, Position: e.Position, Title: e.Title,
			Year: e.Year, Overview: e.Overview, PosterPath: e.PosterPath, BackdropPath: e.BackdropPath,
			TrailerKey: e.TrailerKey, State: e.State, RefreshedAt: adminSyncInstant(e.LastCheckedAt),
		}
		if e.ReleaseDate != nil {
			row.ReleaseDate = e.ReleaseDate.Format("2006-01-02")
		}
		out = append(out, row)
	}
	return out
}

func adminAnticipatedOutput(entries []anticipated.Title) *AdminAnticipatedOutput {
	return &AdminAnticipatedOutput{
		ETag: anticipatedETag(entries),
		Body: AdminAnticipatedList{Entries: adminAnticipatedTitles(entries), Max: anticipated.MaxEntries},
	}
}

func (reg *Registry) getAdminAnticipated(ctx context.Context, _ *struct{}) (*AdminAnticipatedOutput, error) {
	s, p := reg.anticipatedService()
	if p != nil {
		return nil, p
	}
	entries, err := s.List(ctx)
	if err != nil {
		return nil, serviceProblem(err)
	}
	return adminAnticipatedOutput(entries), nil
}

func (reg *Registry) updateAdminAnticipated(ctx context.Context, in *AdminAnticipatedUpdateInput) (*AdminAnticipatedOutput, error) {
	s, p := reg.anticipatedService()
	if p != nil {
		return nil, p
	}
	current, err := s.List(ctx)
	if err != nil {
		return nil, serviceProblem(err)
	}
	// A stale If-Match means another admin saved first: refuse rather than clobber.
	if want := strings.TrimSpace(in.IfMatch); want != "" && want != "*" && want != anticipatedETag(current) {
		return nil, NewProblem(TypePreconditionFailed, "The anticipated list changed; reload before saving.").
			WithHeader("ETag", anticipatedETag(current))
	}
	inputs := make([]anticipated.Input, 0, len(in.Body.Entries))
	for _, e := range in.Body.Entries {
		inputs = append(inputs, anticipated.Input{MediaType: e.MediaType, TMDBID: e.TMDBID})
	}
	entries, err := s.Replace(ctx, inputs)
	if err != nil {
		return nil, anticipatedProblem(err)
	}
	return adminAnticipatedOutput(entries), nil
}

func (reg *Registry) searchAdminAnticipated(ctx context.Context, in *AdminAnticipatedSearchInput) (*AdminAnticipatedSearchOutput, error) {
	s, p := reg.anticipatedService()
	if p != nil {
		return nil, p
	}
	results, err := s.Search(ctx, in.MediaType, in.Query)
	if err != nil {
		return nil, serviceProblem(err)
	}
	out := AdminAnticipatedSearchOutput{Body: Collection[AdminAnticipatedSearchResult]{
		Items: make([]AdminAnticipatedSearchResult, 0, len(results)),
	}}
	for _, r := range results {
		out.Body.Items = append(out.Body.Items, AdminAnticipatedSearchResult{
			MediaType: r.MediaType, TMDBID: r.TMDBID, Title: r.Title, Year: r.Year,
			ReleaseDate: r.ReleaseDate, PosterPath: r.PosterPath, BackdropPath: r.BackdropPath,
			Overview: r.Overview, VoteAverage: r.VoteAverage,
		})
	}
	return &out, nil
}

func (reg *Registry) refreshAdminAnticipated(ctx context.Context, _ *struct{}) (*AdminAnticipatedRefreshOutput, error) {
	s, p := reg.anticipatedService()
	if p != nil {
		return nil, p
	}
	refreshed, err := s.Refresh(ctx, anticipated.MaxEntries)
	out := new(AdminAnticipatedRefreshOutput)
	out.Body.Refreshed = refreshed
	if err != nil && refreshed == 0 {
		return nil, serviceProblem(err)
	}
	return out, nil
}

