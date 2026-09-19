// watchdog_test.go pins the watchdog daemon's pure seams — planSessionDiff and sessionsAreIdle —
// against no tmux server and no filesystem at all, table-driven.

package reedcli

import (
	"errors"
	"sort"
	"testing"
)

func TestPlanSessionDiff(t *testing.T) {
	tests := []struct {
		name         string
		live         []string
		known        map[string]watchedSession
		wantAppeared []string
		wantDeparted []string
	}{
		{
			name:         "empty to populated",
			live:         []string{"alpha", "beta"},
			known:        map[string]watchedSession{},
			wantAppeared: []string{"alpha", "beta"},
			wantDeparted: nil,
		},
		{
			name: "populated to empty",
			live: nil,
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: nil,
			wantDeparted: []string{"alpha", "beta"},
		},
		{
			name: "partial overlap",
			live: []string{"alpha", "gamma"},
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: []string{"gamma"},
			wantDeparted: []string{"beta"},
		},
		{
			name: "no change",
			live: []string{"alpha", "beta"},
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: nil,
			wantDeparted: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAppeared, gotDeparted := planSessionDiff(tt.live, tt.known)
			sort.Strings(gotAppeared)
			sort.Strings(gotDeparted)
			if !equalStringSlices(gotAppeared, tt.wantAppeared) {
				t.Errorf("planSessionDiff() appeared = %v; want %v", gotAppeared, tt.wantAppeared)
			}
			if !equalStringSlices(gotDeparted, tt.wantDeparted) {
				t.Errorf("planSessionDiff() departed = %v; want %v", gotDeparted, tt.wantDeparted)
			}
		})
	}
}

// equalStringSlices treats a nil slice and an empty slice as equal, since planSessionDiff never
// distinguishes "no names" from "an empty allocated slice of names".
func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSessionsAreIdle(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		err   error
		want  bool
	}{
		{
			name:  "non-empty listing, no error",
			names: []string{"alpha"},
			err:   nil,
			want:  false,
		},
		{
			name:  "empty listing, no error",
			names: nil,
			err:   nil,
			want:  true,
		},
		{
			name:  "non-empty listing, error",
			names: []string{"alpha"},
			err:   errors.New("stale listing"),
			want:  true,
		},
		{
			name:  "empty listing, error",
			names: nil,
			err:   errors.New("no server running"),
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionsAreIdle(tt.names, tt.err)
			if got != tt.want {
				t.Errorf("sessionsAreIdle(%v, %v) = %v; want %v", tt.names, tt.err, got, tt.want)
			}
		})
	}
}
