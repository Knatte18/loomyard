// proctree_test.go table-tests the pure /proc process-tree helpers in
// proctree.go: parseStatPPID's stat-line parsing (including the
// space-and-paren comm edge case), descendantClosure's fixed-point walk
// (including a missing-parent, a reparent-to-init, and a cycle), and
// matchSocketCmdlines' argv matcher (including both near-miss shapes). These
// are the TDD surface for the batch: the OS-suffixed seams that call them
// (proctree_linux.go, proctree_windows.go) are compile-checked only, never
// run on this host.

package muxengine

import (
	"sort"
	"testing"
)

func TestParseStatPPID(t *testing.T) {
	tests := []struct {
		name    string
		stat    string
		want    int
		wantErr bool
	}{
		{
			name: "comm with embedded space and paren",
			stat: "1234 (a) b) S 42 1234 1234 0 -1 4194304 100 0 0 0",
			want: 42,
		},
		{
			name: "normal comm",
			stat: "99 (bash) S 1 99 99 0 -1 4194304 100 0 0 0",
			want: 1,
		},
		{
			name:    "malformed: no closing paren",
			stat:    "1234 bash S 42",
			wantErr: true,
		},
		{
			name:    "malformed: too few fields after comm",
			stat:    "1234 (bash) S",
			wantErr: true,
		},
		{
			name:    "malformed: non-numeric ppid",
			stat:    "1234 (bash) S notanumber 1234",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatPPID(tt.stat)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseStatPPID(%q): expected error, got nil", tt.stat)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStatPPID(%q): unexpected error: %v", tt.stat, err)
			}
			if got != tt.want {
				t.Errorf("parseStatPPID(%q) = %d; want %d", tt.stat, got, tt.want)
			}
		})
	}
}

func TestDescendantClosure(t *testing.T) {
	tests := []struct {
		name       string
		pidToPPID  map[int]int
		roots      []int
		wantSorted []int
	}{
		{
			name: "straight chain",
			pidToPPID: map[int]int{
				2: 1,
				3: 2,
				4: 3,
			},
			roots:      []int{1},
			wantSorted: []int{1, 2, 3, 4},
		},
		{
			name: "pid whose ppid is missing from the map",
			pidToPPID: map[int]int{
				2: 1,
				// pid 4's parent (99) never appears as a key in the map at
				// all — e.g. a process that exited between the two /proc
				// reads that built pidToPPID and its roots snapshot. That
				// must not be fatal: 4 is simply dropped from the walk since
				// 99 is never absorbed.
				4: 99,
			},
			roots:      []int{1},
			wantSorted: []int{1, 2},
		},
		{
			name: "pid re-parented to 1",
			pidToPPID: map[int]int{
				5: 1,
				6: 5,
			},
			roots:      []int{1},
			wantSorted: []int{1, 5, 6},
		},
		{
			name: "self/cycle guard",
			pidToPPID: map[int]int{
				7: 7,
				8: 9,
				9: 8,
			},
			roots:      []int{1},
			wantSorted: []int{1},
		},
		{
			name:       "root-only, empty map",
			pidToPPID:  map[int]int{},
			roots:      []int{42},
			wantSorted: []int{42},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := descendantClosure(tt.pidToPPID, tt.roots)
			sort.Ints(got)
			if len(got) != len(tt.wantSorted) {
				t.Fatalf("descendantClosure(%v, %v) = %v; want %v", tt.pidToPPID, tt.roots, got, tt.wantSorted)
			}
			for i := range tt.wantSorted {
				if got[i] != tt.wantSorted[i] {
					t.Errorf("descendantClosure(%v, %v) = %v; want %v", tt.pidToPPID, tt.roots, got, tt.wantSorted)
					break
				}
			}
		})
	}
}

func TestMatchSocketCmdlines(t *testing.T) {
	tests := []struct {
		name   string
		procs  []ProcCmdline
		binary string
		socket string
		want   []int
	}{
		{
			name: "exact match, absolute path base-name",
			procs: []ProcCmdline{
				{PID: 100, Argv: []string{"/usr/bin/tmux", "-L", "hub1", "new-session"}},
			},
			binary: "tmux",
			socket: "hub1",
			want:   []int{100},
		},
		{
			name: "near-miss: different socket value",
			procs: []ProcCmdline{
				{PID: 101, Argv: []string{"tmux", "-L", "hub2", "new-session"}},
			},
			binary: "tmux",
			socket: "hub1",
			want:   nil,
		},
		{
			name: "near-miss: binary present without -L",
			procs: []ProcCmdline{
				{PID: 102, Argv: []string{"tmux", "new-session"}},
			},
			binary: "tmux",
			socket: "hub1",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchSocketCmdlines(tt.procs, tt.binary, tt.socket)
			if len(got) != len(tt.want) {
				t.Fatalf("matchSocketCmdlines(...) = %v; want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("matchSocketCmdlines(...) = %v; want %v", got, tt.want)
				}
			}
		})
	}
}
