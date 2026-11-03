package requests

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeQualities(t *testing.T) {
	tests := []struct {
		name    string
		in      []Quality
		want    []Quality
		wantErr bool
	}{
		{name: "none defaults to HD", in: nil, want: []Quality{Quality1080p}},
		{name: "HD only", in: []Quality{Quality1080p}, want: []Quality{Quality1080p}},
		{name: "4K only", in: []Quality{Quality2160p}, want: []Quality{Quality2160p}},
		{
			name: "both ordered HD first",
			in:   []Quality{Quality2160p, Quality1080p},
			want: []Quality{Quality1080p, Quality2160p},
		},
		{name: "deduped", in: []Quality{Quality1080p, Quality1080p}, want: []Quality{Quality1080p}},
		{name: "unknown refused", in: []Quality{Quality("480p")}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeQualities(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeQualities(%v) = %v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeQualities(%v): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("NormalizeQualities(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestQualitiesOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b []Quality
		want bool
	}{
		{name: "shared HD", a: []Quality{Quality1080p}, b: []Quality{Quality1080p, Quality2160p}, want: true},
		{name: "shared 4K", a: []Quality{Quality2160p}, b: []Quality{Quality1080p, Quality2160p}, want: true},
		{name: "disjoint", a: []Quality{Quality1080p}, b: []Quality{Quality2160p}, want: false},
		{name: "empty", a: nil, b: []Quality{Quality1080p}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := QualitiesOverlap(tc.a, tc.b); got != tc.want {
				t.Fatalf("QualitiesOverlap(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// A non-admin request is always HD only, whatever the playback ceiling.
func TestCreateRequestNonAdminIsHDOnly(t *testing.T) {
	store := newFakeStore()
	store.settings.RequestsEnabled = true
	service := newTestService(store)
	service.SetEntitlementResolver(fixedCeiling{q: "2160p"})

	req, err := service.CreateRequest(context.Background(), testViewer(1), fightClubInput)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	want := []Quality{Quality1080p}
	if len(store.created) != 1 {
		t.Fatalf("stored creations = %d, want 1", len(store.created))
	}
	if got := store.created[0].Input.Qualities; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored qualities = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(req.RequestedQualities, want) {
		t.Fatalf("request qualities = %v, want %v", req.RequestedQualities, want)
	}
}

// A non-admin may not ask for 4K, even if they send it.
func TestCreateRequestNonAdminCannotChoose4K(t *testing.T) {
	store := newFakeStore()
	store.settings.RequestsEnabled = true
	service := newTestService(store)

	in := fightClubInput
	in.Qualities = []Quality{Quality1080p, Quality2160p}
	_, err := service.CreateRequest(context.Background(), testViewer(1), in)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
	if len(store.created) != 0 {
		t.Fatalf("created requests = %d, want 0", len(store.created))
	}
}

// An admin's request stores the qualities they chose; no choice means HD,
// plus 4K when the server-wide force-dual switch is on.
func TestCreateRequestAdminQualityChoice(t *testing.T) {
	admin := Viewer{UserID: 1, ProfileID: "profile-1", IsAdmin: true}
	tests := []struct {
		name      string
		chosen    []Quality
		forceDual bool
		want      []Quality
	}{
		{name: "no choice is HD", want: []Quality{Quality1080p}},
		{name: "explicit both", chosen: []Quality{Quality1080p, Quality2160p}, want: []Quality{Quality1080p, Quality2160p}},
		{name: "explicit 4K only", chosen: []Quality{Quality2160p}, want: []Quality{Quality2160p}},
		{name: "force dual adds 4K", forceDual: true, want: []Quality{Quality1080p, Quality2160p}},
		{name: "explicit HD beats force dual", chosen: []Quality{Quality1080p}, forceDual: true, want: []Quality{Quality1080p}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			store.settings.RequestsEnabled = true
			store.settings.ForceDualQuality = tc.forceDual
			service := newTestService(store)

			in := fightClubInput
			in.Qualities = tc.chosen
			if _, err := service.CreateRequest(context.Background(), admin, in); err != nil {
				t.Fatalf("CreateRequest: %v", err)
			}
			if got := store.created[0].Input.Qualities; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("stored qualities = %v, want %v", got, tc.want)
			}
		})
	}
}

// An unknown quality is refused, admin or not.
func TestCreateRequestUnknownQualityRefused(t *testing.T) {
	store := newFakeStore()
	store.settings.RequestsEnabled = true
	service := newTestService(store)

	in := fightClubInput
	in.Qualities = []Quality{Quality("480p")}
	_, err := service.CreateRequest(context.Background(), Viewer{UserID: 1, ProfileID: "p", IsAdmin: true}, in)
	if err == nil {
		t.Fatal("error = nil, want a refusal for an unknown quality")
	}
	if len(store.created) != 0 {
		t.Fatalf("created requests = %d, want 0", len(store.created))
	}
}

// An active request that shares a quality blocks a new one; a disjoint quality
// set coexists (an HD request next to a 4K one).
func TestCreateRequestActiveQualityOverlap(t *testing.T) {
	tests := []struct {
		name   string
		active []Quality
		want   error
	}{
		{name: "HD blocks HD", active: []Quality{Quality1080p}, want: ErrAlreadyRequested},
		{name: "both blocks HD", active: []Quality{Quality1080p, Quality2160p}, want: ErrAlreadyRequested},
		{name: "4K only does not block HD", active: []Quality{Quality2160p}, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			store.settings.RequestsEnabled = true
			store.active[MediaTypeMovie][550] = &Request{
				ID: "req-existing", MediaType: MediaTypeMovie, TMDBID: 550,
				Status: StatusQueued, Outcome: OutcomeActive, RequestedQualities: tc.active,
			}
			service := newTestService(store)

			_, err := service.CreateRequest(context.Background(), testViewer(1), fightClubInput)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
