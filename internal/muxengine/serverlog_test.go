// serverlog_test.go verifies the pure debug_log parsing helper. Untagged: no
// filesystem or process I/O, per Test Tier Purity.

package muxengine

import (
	"reflect"
	"testing"
	"time"
)

func TestDebugLogArgs(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    []string
		wantErr bool
	}{
		{"off", "0", nil, false},
		{"verbose", "1", []string{"-v"}, false},
		{"veryVerbose", "2", []string{"-vv"}, false},
		{"whitespaceTrimmed", " 1 ", []string{"-v"}, false},
		{"empty", "", nil, true},
		{"invalid", "3", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := debugLogArgs(tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("debugLogArgs(%q) = %v, nil; want error", tt.level, got)
				}
				wantMsg := `invalid debug_log "` + tt.level + `": must be 0, 1 or 2`
				if err.Error() != wantMsg {
					t.Errorf("debugLogArgs(%q) error = %q; want %q", tt.level, err.Error(), wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("debugLogArgs(%q) unexpected error: %v", tt.level, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("debugLogArgs(%q) = %v; want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestPlanLogPrune(t *testing.T) {
	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		names  []string
		ages   []time.Duration // subtracted from base to build mtimes; 0 = newest
		keep   int
		wantDe []string
	}{
		{
			name:   "fewerThanKeep",
			names:  []string{"a.log", "b.log"},
			ages:   []time.Duration{0, time.Minute},
			keep:   3,
			wantDe: nil,
		},
		{
			name:   "exactlyKeep",
			names:  []string{"a.log", "b.log", "c.log"},
			ages:   []time.Duration{0, time.Minute, 2 * time.Minute},
			keep:   3,
			wantDe: nil,
		},
		{
			name:   "moreThanKeep",
			names:  []string{"a.log", "b.log", "c.log", "d.log"},
			ages:   []time.Duration{0, time.Minute, 2 * time.Minute, 3 * time.Minute},
			keep:   2,
			wantDe: []string{"c.log", "d.log"},
		},
		{
			name:   "ties",
			names:  []string{"a.log", "b.log", "c.log"},
			ages:   []time.Duration{0, 0, 0},
			keep:   1,
			wantDe: []string{"b.log", "c.log"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mtimes := make([]time.Time, len(tt.ages))
			for i, age := range tt.ages {
				mtimes[i] = base.Add(-age)
			}
			got := planLogPrune(tt.names, mtimes, tt.keep)
			if !reflect.DeepEqual(got, tt.wantDe) {
				t.Errorf("planLogPrune(%v, _, %d) = %v; want %v", tt.names, tt.keep, got, tt.wantDe)
			}
		})
	}
}
