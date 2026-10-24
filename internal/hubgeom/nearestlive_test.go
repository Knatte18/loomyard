//go:build tier0

// nearestlive_test.go covers the walk behind NearestLiveOrch over told reads, so no filesystem is touched.

package hubgeom

import (
	"errors"
	"testing"
)

func TestNearestLive(t *testing.T) {
	t.Parallel()

	errLive := errors.New("probe failed")
	type parentRead struct {
		parent string
		found  bool
	}
	tests := []struct {
		name    string
		start   string
		parents map[string]parentRead
		live    map[string]bool
		liveErr map[string]error
		want    string
		wantErr error
	}{
		{
			name:    "live start answers itself",
			start:   "b",
			parents: map[string]parentRead{"b": {"a", true}},
			live:    map[string]bool{"b": true, "a": true},
			want:    "b",
		},
		{
			name:    "stopped beam answers the live beam above it",
			start:   "b",
			parents: map[string]parentRead{"b": {"a", true}, "a": {"", true}},
			live:    map[string]bool{"a": true},
			want:    "a",
		},
		{
			name:    "no live orch answers the prime",
			start:   "b",
			parents: map[string]parentRead{"b": {"a", true}, "a": {"", true}},
			want:    "",
		},
		{
			name:    "gone parent worktree answers the prime",
			start:   "b",
			parents: map[string]parentRead{"b": {"", false}},
			live:    map[string]bool{"a": true},
			want:    "",
		},
		{
			name:    "cycle answers the prime",
			start:   "a",
			parents: map[string]parentRead{"a": {"b", true}, "b": {"a", true}},
			want:    "",
		},
		{
			name:  "prime start is never probed",
			start: "",
			liveErr: map[string]error{
				"": errLive,
			},
			want: "",
		},
		{
			name:    "predicate error is returned",
			start:   "b",
			parents: map[string]parentRead{"b": {"a", true}},
			liveErr: map[string]error{"b": errLive},
			wantErr: errLive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := nearestLive(tt.start,
				func(worktree string) (string, bool, error) {
					read := tt.parents[worktree]
					return read.parent, read.found, nil
				},
				func(worktree string) (bool, error) { return tt.live[worktree], tt.liveErr[worktree] },
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("nearestLive(%q) error = %v; want %v", tt.start, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("nearestLive(%q) = %q; want %q", tt.start, got, tt.want)
			}
		})
	}
}
