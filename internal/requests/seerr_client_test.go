package requests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// seerrRecorder stands in for Seerr: it answers the reads and writes the client
// makes and records every call, so a test can assert the exact endpoints, query
// parameters and credentials.
type seerrRecorder struct {
	mu      sync.Mutex
	calls   []string
	lastKey string
	// mediaID is the media entry GET /request/{id} reports; 0 means Seerr names
	// none.
	mediaID int
	is4K    bool
	missing bool
}

func (s *seerrRecorder) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := r.URL.Path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	s.calls = append(s.calls, r.Method+" "+u)
	s.lastKey = r.Header.Get("X-Api-Key")
}

func (s *seerrRecorder) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *seerrRecorder) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/request/", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.missing {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := map[string]any{"id": 1495, "is4k": s.is4K}
		if s.mediaID > 0 {
			body["media"] = map[string]any{"id": s.mediaID}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/api/v1/media/", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func seerrTestIntegration(url string) Integration {
	return Integration{ID: "seerr", Name: "Seer", Enabled: true, CapabilityID: seerrCapabilityID, BaseURL: url, APIKeyRef: "test-key"}
}

func seerrTestTarget() Target {
	return Target{ExternalID: "1495", Quality: Quality2160p, IntegrationID: "seerr", ExternalStatus: "approved"}
}

// A delete asks Seerr to drop the title from Radarr or Sonarr and then drops the
// request, and it never deletes Seerr's media row: one row holds both the HD and
// 4K variants, so deleting it for one takes the other's media id with it and
// strands its download.
func TestSeerrDeleteRemovesTheTitleAndKeepsTheMediaRow(t *testing.T) {
	rec := &seerrRecorder{mediaID: 1458, is4K: true}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := NewSeerrClient(srv.Client())
	if err := client.DeleteTarget(context.Background(), seerrTestIntegration(srv.URL), seerrTestTarget(), DeleteOptions{RemoveMedia: true}); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}

	want := []string{
		"GET /api/v1/request/1495",
		"DELETE /api/v1/media/1458/file?is4k=true",
		"DELETE /api/v1/request/1495",
	}
	if got := rec.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if rec.lastKey != "test-key" {
		t.Fatalf("X-Api-Key = %q, want the connection's key", rec.lastKey)
	}
}

// A delete that cannot name the media entry refuses instead of removing only the
// request: the download would stay in Radarr or Sonarr with nothing left in Seerr
// to point at it.
func TestSeerrDeleteRefusesWhenSeerrNamesNoMediaEntry(t *testing.T) {
	rec := &seerrRecorder{} // GET reports no media entry
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := NewSeerrClient(srv.Client())
	err := client.DeleteTarget(context.Background(), seerrTestIntegration(srv.URL), seerrTestTarget(), DeleteOptions{RemoveMedia: true})
	if err == nil {
		t.Fatal("DeleteTarget succeeded with no media entry; want an error")
	}
	if !strings.Contains(err.Error(), "no media entry") {
		t.Fatalf("error = %v, want it to name the missing media entry", err)
	}
	if got := rec.snapshot(); !slices.Equal(got, []string{"GET /api/v1/request/1495"}) {
		t.Fatalf("calls = %v, want the read alone: nothing destructive may run", got)
	}
}

// Silo-only never reaches the download service at all.
func TestSeerrDeleteSiloOnlyTouchesNothing(t *testing.T) {
	rec := &seerrRecorder{mediaID: 1458}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := NewSeerrClient(srv.Client())
	if err := client.DeleteTarget(context.Background(), seerrTestIntegration(srv.URL), seerrTestTarget(), DeleteOptions{SiloOnly: true}); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("calls = %v, want none", got)
	}
}

// The reconcile pass reads a 404 as "Seerr no longer has it".
func TestSeerrTargetExistsReadsA404AsGone(t *testing.T) {
	rec := &seerrRecorder{missing: true}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := NewSeerrClient(srv.Client())
	exists, err := client.TargetExists(context.Background(), seerrTestIntegration(srv.URL), seerrTestTarget())
	if err != nil {
		t.Fatalf("TargetExists: %v", err)
	}
	if exists {
		t.Fatal("a 404 must read as gone")
	}
}
