// validateanchor_test.go covers ValidateAnchorRel, the single validator both sides of the
// .lyx-anchor marker run values through.
// It spawns nothing — the function is pure string/path math — so this file stays untagged, in
// Tier 1.

package lyxcwd_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestValidateAnchorRel(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to repo root", raw: "", want: "."},
		{name: "whitespace only defaults to repo root", raw: "  \n", want: "."},
		{name: "explicit dot", raw: ".", want: "."},
		{name: "plain subdirectory", raw: "backend", want: "backend"},
		{name: "nested subdirectory", raw: "services/backend", want: "services/backend"},
		{name: "trailing slash is cleaned", raw: "backend/", want: "backend"},
		{name: "interior dotdot is cleaned away", raw: "backend/../frontend", want: "frontend"},
		{name: "surrounding whitespace is trimmed", raw: "  backend\n", want: "backend"},
		{name: "absolute is rejected", raw: "/backend", wantErr: true},
		{name: "escaping one level is rejected", raw: "..", wantErr: true},
		{name: "escaping two levels is rejected", raw: "../..", wantErr: true},
		{name: "escaping via a segment is rejected", raw: "backend/../..", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lyxcwd.ValidateAnchorRel(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateAnchorRel(%q) = %q, nil; want an error", tt.raw, got)
				}
				if !errors.Is(err, lyxcwd.ErrInvalidAnchor) {
					t.Errorf("ValidateAnchorRel(%q) error = %v; want wrapped ErrInvalidAnchor", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAnchorRel(%q) error = %v; want nil", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ValidateAnchorRel(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}
}
