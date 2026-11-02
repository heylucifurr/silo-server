package anticipated

import (
	"strings"
	"testing"
)

func TestValidateInputs(t *testing.T) {
	valid := []Input{{MediaType: MediaMovie, TMDBID: 550}, {MediaType: MediaSeries, TMDBID: 1399}}

	// A 51-entry list, one over the bound.
	tooMany := make([]Input, 0, MaxEntries+1)
	for i := 0; i < MaxEntries+1; i++ {
		tooMany = append(tooMany, Input{MediaType: MediaMovie, TMDBID: 1000 + i})
	}

	cases := []struct {
		name    string
		in      []Input
		wantErr string // substring; "" means no error
	}{
		{name: "valid", in: valid, wantErr: ""},
		{name: "empty is valid", in: nil, wantErr: ""},
		{name: "bad media type", in: []Input{{MediaType: "book", TMDBID: 1}}, wantErr: "entry 0: media_type"},
		{name: "zero tmdb id", in: []Input{{MediaType: MediaMovie, TMDBID: 0}}, wantErr: "entry 0: tmdb_id"},
		{name: "duplicate pair", in: []Input{{MediaType: MediaMovie, TMDBID: 550}, {MediaType: MediaMovie, TMDBID: 550}}, wantErr: "entry 1"},
		{name: "same id different type is not a duplicate", in: []Input{{MediaType: MediaMovie, TMDBID: 550}, {MediaType: MediaSeries, TMDBID: 550}}, wantErr: ""},
		{name: "too many", in: tooMany, wantErr: "at most"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInputs(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateInputs() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateInputs() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateInputs() error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
