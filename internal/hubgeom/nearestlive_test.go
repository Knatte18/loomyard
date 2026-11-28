// nearestlive_test.go covers the walks behind NearestLiveOrch and OrchChain over told reads, so no filesystem is touched.

package hubgeom

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChainWorktrees(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	errRead := errors.New("origin read failed")
	type parentRead struct {
		parent string
		found  bool
		err    error
	}
	tests := []struct {
		name    string
		start   string
		parents map[string]parentRead
		want    []string
		wantErr error
	}{
		{
			name:    "a member lists its beam and the prime",
			start:   "m",
			parents: map[string]parentRead{"m": {parent: "b", found: true}, "b": {parent: "", found: true}},
			want:    []string{"m", "b", ""},
		},
		{
			name:    "a gone parent worktree ends the walk at the prime",
			start:   "m",
			parents: map[string]parentRead{"m": {}},
			want:    []string{"m", ""},
		},
		{
			name:    "a cycle ends the walk at the prime",
			start:   "a",
			parents: map[string]parentRead{"a": {parent: "b", found: true}, "b": {parent: "a", found: true}},
			want:    []string{"a", "b", ""},
		},
		{
			name:  "the prime lists itself alone",
			start: "",
			want:  []string{""},
		},
		{
			name:    "a read error is returned",
			start:   "m",
			parents: map[string]parentRead{"m": {parent: "b", found: true}, "b": {err: errRead}},
			wantErr: errRead,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := chainWorktrees(tt.start, func(worktree string) (string, bool, error) {
				read := tt.parents[worktree]
				return read.parent, read.found, read.err
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("chainWorktrees(%q) error = %v; want %v", tt.start, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("chainWorktrees(%q) = %q; want %q", tt.start, got, tt.want)
			}
		})
	}
}

func TestNearestLive(t *testing.T) {
	tiergate.Require(t, 0)
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
