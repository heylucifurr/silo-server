package apiv2

import (
	"context"
	"net/http"
	"testing"

	"github.com/Silo-Server/silo-server/internal/anticipated"
)

// fakeAnticipated is the curation service with the validation the real one does.
type fakeAnticipated struct {
	entries []anticipated.Title
	search  []anticipated.Result
	writes  int
}

func (f *fakeAnticipated) List(context.Context) ([]anticipated.Title, error) { return f.entries, nil }

func (f *fakeAnticipated) Replace(_ context.Context, in []anticipated.Input) ([]anticipated.Title, error) {
	if err := anticipated.ValidateInputs(in); err != nil {
		return nil, err
	}
	f.writes++
	out := make([]anticipated.Title, 0, len(in))
	for i, e := range in {
		out = append(out, anticipated.Title{
			MediaType: e.MediaType, TMDBID: e.TMDBID, Position: i,
			Title: "Stored", State: anticipated.StateActive,
		})
	}
	f.entries = out
	return out, nil
}

func (f *fakeAnticipated) Search(context.Context, string, string) ([]anticipated.Result, error) {
	return f.search, nil
}

func (f *fakeAnticipated) Refresh(context.Context, int) (int, error) { return 2, nil }

func TestAdminAnticipatedList(t *testing.T) {
	f := &fakeAnticipated{entries: []anticipated.Title{
		{MediaType: "movie", TMDBID: 1083381, Position: 0, Title: "Avengers: Doomsday", State: anticipated.StateActive},
	}}
	deps := pilotDeps(nil, nil)
	deps.Anticipated = f
	h := NewHandler(deps)
	path := Prefix + "/admin/anticipated"

	// The curation surface is admin-only.
	requireProblem(t, do(t, h, http.MethodGet, path, "", requestOwner), TypePermissionDenied)

	read := do(t, h, http.MethodGet, path, "", actingRequestAdmin)
	var got AdminAnticipatedList
	decodeBody(t, read.Body, &got)
	tag := read.Header().Get("ETag")
	if read.Code != http.StatusOK || tag == "" || len(got.Entries) != 1 ||
		got.Entries[0].TMDBID != 1083381 || got.Max != anticipated.MaxEntries {
		t.Fatalf("%d %s", read.Code, read.Body.String())
	}

	// A stale If-Match is refused and nothing is written.
	body := `{"entries":[{"media_type":"movie","tmdb_id":1}]}`
	requireProblem(t, do(t, h, http.MethodPut, path, body, with(actingRequestAdmin, "If-Match", `"stale"`)), TypePreconditionFailed)
	if f.writes != 0 {
		t.Fatal("a refused write took effect")
	}

	// A duplicate entry is refused naming the admin's input, not the server.
	dup := `{"entries":[{"media_type":"movie","tmdb_id":1},{"media_type":"movie","tmdb_id":1}]}`
	requireProblem(t, do(t, h, http.MethodPut, path, dup, with(actingRequestAdmin, "If-Match", tag)), TypeValidationFailed)

	// A matching If-Match saves and moves the tag on.
	saved := do(t, h, http.MethodPut, path, body, with(actingRequestAdmin, "If-Match", tag))
	if saved.Code != http.StatusOK || f.writes != 1 || saved.Header().Get("ETag") == tag {
		t.Fatalf("%d %s", saved.Code, saved.Body.String())
	}
}
