// junctionnames_test.go — unit tests for junctionNames, filterHubReserved, BoardDir, HubPath and IsReservedHubName.
//
// Package fabricengine, not fabricengine_test, so it can call the unexported junctionNames and filterHubReserved.
// Hermetic: no git spawn, so this stays a Tier-1 unit test although other files in the package spawn git.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	reserved := HubReservedNames()

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

	// HubReservedNames() must contain the Bolt checkout, _portals and _launchers:
	// the junction-wiring block set the cases above rely on being dropped.
	for _, want := range []string{BoltDirName, "_portals", "_launchers"} {
		found := false
		for _, r := range reserved {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("HubReservedNames() = %v; want it to contain %q", reserved, want)
		}
	}

	// .lym must not be in HubReservedNames().
	// That would make filterHubReserved drop it from the wired names, so the per-worktree junction is never created.
	// It would also make scanOnDiskJunctionNames skip it, so Unwire's sweep and applyStaleRemoval could never see it.
	for _, r := range reserved {
		if r == ".lym" {
			t.Errorf("HubReservedNames() = %v; want it to NEVER contain %q", reserved, ".lym")
		}
	}
}

// TestBoardDir verifies that BoardDir and BoltDir join hub with the Bolt directory name.
//
//testtiming:keep BoardDir and BoltDir joining the hub with the Bolt directory name; coverage of their blocks by other tests does not show an assertion of this
func TestBoardDir(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		dir  func(string) string
		hub  string
		want string
	}{
		{
			name: "simple hub",
			dir:  BoardDir,
			hub:  "/h",
			want: filepath.Join("/h", BoardDirName),
		},
		{
			name: "nested hub",
			dir:  BoardDir,
			hub:  "/repos/loomyard-LYMHUB",
			want: filepath.Join("/repos/loomyard-LYMHUB", BoardDirName),
		},
		{
			name: "Bolt directory equals the lymcwd name and the board directory",
			dir:  BoltDir,
			hub:  "/h",
			want: filepath.Join("/h", lymcwd.BoltDirName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dir(tt.hub)
			if got != tt.want {
				t.Errorf("dir(%q) = %q; want %q", tt.hub, got, tt.want)
			}
			if board := BoardDir(tt.hub); got != board {
				t.Errorf("dir(%q) = %q; want it equal to BoardDir(%q) = %q", tt.hub, got, tt.hub, board)
			}
		})
	}
}

// TestHubPath verifies that HubPath joins parent and name with HubSuffix.
// The "simple repo name" case pins the literal -LYMHUB suffix, so a change to HubSuffix fails this test.
// The "nested parent" case checks that HubPath composes from HubSuffix rather than from a hardcoded string.
//
//testtiming:keep HubPath joining parent and name with HubSuffix, pinning the literal -LYMHUB suffix; coverage of its blocks by other tests does not show an assertion of this
func TestHubPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		parent   string
		repoName string
		want     string
	}{
		{
			name:     "simple repo name",
			parent:   "/repos",
			repoName: "loomyard",
			want:     filepath.Join("/repos", "loomyard-LYMHUB"),
		},
		{
			name:     "nested parent",
			parent:   "/home/user/code",
			repoName: "myproject",
			want:     filepath.Join("/home/user/code", "myproject"+HubSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HubPath(tt.parent, tt.repoName)
			if got != tt.want {
				t.Errorf("HubPath(%q, %q) = %q; want %q", tt.parent, tt.repoName, got, tt.want)
			}
		})
	}
}

// TestIsReservedHubName verifies the reserved hub-entry name predicate that slug validation uses.
// Every geometry-owned hub-level name is reserved: the union of HubReservedNames() and junctionNames.
// Ordinary slugs and near-misses are not.
//
//testtiming:keep the reserved hub-entry predicate slug validation gates on, over every geometry-owned name and near-misses; coverage of its blocks by other tests does not show an assertion of this
func TestIsReservedHubName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	junctionNames := []string{}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lym dir", "_lym", true},
		{"raddle dir", "_raddle", false},
		{"board dir", BoltDirName, true},
		{"portals dir", "_portals", true},
		{"launchers dir", "_launchers", true},
		{"ordinary slug", "my-task", false},
		{"underscore-prefixed but unreserved", "_mytask", false},
		{"compound near-miss", "_boltroom", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedHubName(tt.input, junctionNames); got != tt.want {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want %v", tt.input, junctionNames, got, tt.want)
			}
		})
	}

	// The hub-structural tokens stay reserved when junctionNames is empty, as it is for the default pathspec.
	// A worktree slug must never claim the Bolt checkout, _portals, or _launchers, whatever junctionNames holds.
	// This subtest is the only coverage of that invariant for an empty junctionNames.
	t.Run("hub-structural tokens reserved for empty junctionNames", func(t *testing.T) {
		t.Parallel()

		for _, hubStructural := range []string{BoltDirName, "_portals", "_launchers"} {
			if got := IsReservedHubName(hubStructural, []string{}); !got {
				t.Errorf("IsReservedHubName(%q, []) = %v; want true", hubStructural, got)
			}
		}
	})

	// A name present only in the caller-supplied junctionNames, not in HubReservedNames(), must be reported reserved.
	t.Run("junction-only name is reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_custom", []string{"_custom"}); !got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want true", "_custom", []string{"_custom"}, got)
		}
	})

	// A name absent from both HubReservedNames() and junctionNames must not be reserved.
	t.Run("unrelated name is not reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_other", []string{"_custom"}); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "_other", []string{"_custom"}, got)
		}
	})

	// With junctionNames empty, as for the default pathspec, four names stay reserved:
	// _lym via structuralCommittedDirs, and the Bolt checkout, _portals and _launchers via HubReservedNames().
	// `.lym` is also reserved, via structuralNeverCommittedDirs, but this list does not include it.
	// The check is a positive set, not an exhaustive one over every reserved name.
	t.Run("default pathspec union reserves exactly four names", func(t *testing.T) {
		t.Parallel()

		wantReserved := []string{"_lym", BoltDirName, "_portals", "_launchers"}
		for _, name := range wantReserved {
			if got := IsReservedHubName(name, junctionNames); !got {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want true", name, junctionNames, got)
			}
		}
		if got := IsReservedHubName("not-reserved", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "not-reserved", junctionNames, got)
		}
	})

	// _raddle has no hub-level presence: its design lives at the anchor, under `_lym/raddle/`.
	// A worktree slug named _raddle is therefore not reserved by IsReservedHubName.
	t.Run("raddle slug is no longer reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_raddle", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "_raddle", junctionNames, got)
		}
	})
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
			tt.makeBoard(t, BoardDir(hub))
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
				wantBoard = BoardDir(hub)
			}
			if got.Anchor != loc.AnchorPath() || got.Board != wantBoard || got.Records != wantWeft {
				t.Errorf("ConfigDirs = %+v; want Anchor %q, Board %q, Records %q", got, loc.AnchorPath(), wantBoard, wantWeft)
			}
		})
	}
}
