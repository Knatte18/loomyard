// junctionnames_test.go — unit tests for junctionNames, filterHubReserved and ConfigDirs.
//
// Package fabricengine, not fabricengine_test, so it can call the unexported junctionNames and filterHubReserved.
// Hermetic: no git spawn, so this stays a Tier-1 unit test although other files in the package spawn git.

package fabricengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestJunctionNames_NoFallbackOnLoadFailure asserts the no-fallback rule:
// a config-load failure at baseDir yields a non-nil error and a nil name slice, never a default of `_lym`.
//
//testtiming:keep a config-load failure surfacing as an error and a nil name slice instead of defaulting to _lym; coverage of its blocks by other tests does not show an assertion of this
func TestJunctionNames_NoFallbackOnLoadFailure(t *testing.T) {
	tiergate.Require(t, 0)
	baseDir := t.TempDir() // no _lym/, so LoadConfig cannot find fabric.yaml

	names, err := junctionNames(configengine.Dirs{Board: baseDir})
	if err == nil {
		t.Fatalf("junctionNames(%q) error = nil; want a config-load error", baseDir)
	}
	if names != nil {
		t.Errorf("junctionNames(%q) names = %v; want nil on error (no fallback default)", baseDir, names)
	}
}

// TestFilterHubReserved covers the reserved-name filter on mixed, all-reserved and no-reserved input.
func TestFilterHubReserved(t *testing.T) {
	tiergate.Require(t, 0)
	reserved := fabriclayout.HubReservedNames()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "MixedDropsReservedKeepsOrder",
			input: append(append([]string{"_lym"}, reserved...), "_other", "_extra"),
			want:  []string{"_lym", "_other", "_extra"},
		},
		{
			name:  "AllReservedYieldsEmpty",
			input: reserved,
			want:  []string{},
		},
		{
			name:  "NoneReservedReturnedUnchanged",
			input: []string{"_lym", "_other", "_extra"},
			want:  []string{"_lym", "_other", "_extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterHubReserved(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("filterHubReserved(%v) = %v; want %v", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("filterHubReserved(%v)[%d] = %q; want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestConfigDirs pins the geometry a location tells a layered load.
// The anchor is always included; the board dir and the weft checkout only when each exists as a directory.
func TestConfigDirs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		makeBoard func(t *testing.T, board string)
		wantBoard bool
		// makeWeft reports whether the weft checkout the location pairs with is created.
		makeWeft bool
	}{
		{
			name: "hub holding a board dir and a weft checkout",
			makeBoard: func(t *testing.T, board string) {
				if err := os.Mkdir(board, 0o755); err != nil {
					t.Fatalf("mkdir board: %v", err)
				}
			},
			wantBoard: true,
			makeWeft:  true,
		},
		{
			name: "hub holding a board dir",
			makeBoard: func(t *testing.T, board string) {
				if err := os.Mkdir(board, 0o755); err != nil {
					t.Fatalf("mkdir board: %v", err)
				}
			},
			wantBoard: true,
		},
		{
			name:      "hub without a board dir",
			makeBoard: func(*testing.T, string) {},
		},
		{
			name: "board path that is a file",
			makeBoard: func(t *testing.T, board string) {
				if err := os.WriteFile(board, []byte("x"), 0o644); err != nil {
					t.Fatalf("write board file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			tt.makeBoard(t, fabriclayout.BoardDir(hub))
			loc := &lymcwd.Location{RepoName: "repo", HubPath: hub, WorktreeName: "main", AnchorRel: "."}
			wantWeft := ""
			if tt.makeWeft {
				wantWeft = RecordsWorktree(loc)
				if err := os.MkdirAll(wantWeft, 0o755); err != nil {
					t.Fatalf("mkdir weft: %v", err)
				}
			}

			got := ConfigDirs(loc)

			wantBoard := ""
			if tt.wantBoard {
				wantBoard = fabriclayout.BoardDir(hub)
			}
			if got.Anchor != loc.AnchorPath() || got.Board != wantBoard || got.Records != wantWeft {
				t.Errorf("ConfigDirs = %+v; want Anchor %q, Board %q, Records %q", got, loc.AnchorPath(), wantBoard, wantWeft)
			}
		})
	}
}
