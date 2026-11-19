// anchorread_test.go pins the marker semantics of a worktree's recorded anchor in Tier 0.
// The own marker wins over the board marker, the stale pre-rename marker hard-errors only when no own marker answers,
// and an absent or empty marker is a fallback.
// The reads are pure file reads with no git spawn, so the untagged suite stays sensitive to a removed guard.
// The integration-tagged anchor_test.go covers the same guard through the full resolvers, but an untagged run never compiles it.

package lymcwd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

func TestReadRecordedAnchor_UnreadableOwnMarkerIsErrorNamingFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const worktreeName = "feat"
	hub := t.TempDir()
	board := boardDir(hub)
	ownMarker := filepath.Join(weftname.SiblingPath(hub, worktreeName), AnchorFileName)
	if err := os.MkdirAll(ownMarker, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", ownMarker, err)
	}
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", board, err)
	}
	if err := os.WriteFile(filepath.Join(board, AnchorFileName), []byte("backend\n"), 0o644); err != nil {
		t.Fatalf("write the board marker: %v", err)
	}

	anchor, found, err := readRecordedAnchor(hub, worktreeName)

	if err == nil || !strings.Contains(err.Error(), ownMarker) {
		t.Errorf("readRecordedAnchor() = (%q, %v, %v); want an error naming %s", anchor, found, err, ownMarker)
	}
}

func TestReadRecordedAnchor_MarkerSemantics(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const worktreeName = "feat"

	tests := []struct {
		name        string
		staleValue  string // content for StaleAnchorFileName at the board root, "" for absent
		boardValue  string // content for AnchorFileName at the board root, "" for absent
		ownValue    string // content for AnchorFileName in the worktree's own records sibling, "" for absent
		rootValue   string // content for AnchorFileName at the worktree's own root, "" for absent
		wantAnchor  string
		wantErr     error
		wantErrFile string // path the error must name, "" for none
		// wantErrAtRoot makes the error-file check read the root marker's path instead of the records sibling's.
		wantErrAtRoot bool
		// wantOneRepository maps a root (worktree, sibling, bolt) to the answer OneRepository gives for it; absent is false.
		wantOneRepository map[string]bool
	}{
		{name: "AbsentAll_IsFallback", wantAnchor: "."},
		{name: "StaleOnly_HardErrors", staleValue: "backend\n", wantErr: ErrStaleAnchorMarker},
		{name: "BoardOnly_Wins", boardValue: "backend\n", wantAnchor: "backend"},
		{name: "BothBoardMarkers_NewWins", staleValue: "old\n", boardValue: "backend\n", wantAnchor: "backend"},
		{name: "EmptyBoardMarker_IsAbsentNotEmptySubpath", boardValue: "   \n", wantAnchor: "."},
		{name: "OwnMarker_WinsOverBoard", boardValue: "backend\n", ownValue: "frontend\n", wantAnchor: "frontend"},
		{name: "OwnMarkerAbsent_FallsBackToBoard", boardValue: "backend\n", wantAnchor: "backend"},
		{name: "OwnMarkerEmpty_FallsBackToBoard", boardValue: "backend\n", ownValue: "  \n", wantAnchor: "backend"},
		{name: "OwnMarkerOnly", ownValue: "frontend\n", wantAnchor: "frontend"},
		{name: "OwnMarkerInvalid_IsErrorNamingFile", boardValue: "backend\n", ownValue: "../out\n", wantErr: ErrInvalidAnchor, wantErrFile: AnchorFileName},
		{name: "StaleBoardMarkerBesideOwnMarker_IsNoError", staleValue: "old\n", ownValue: "frontend\n", wantAnchor: "frontend"},
		{
			name: "RootMarker_WinsOverSiblingAndBoard", rootValue: "backend\n", ownValue: "frontend\n", boardValue: "other\n",
			wantAnchor: "backend", wantOneRepository: map[string]bool{"worktree": true},
		},
		{name: "RootMarkerWithBoltMissing", rootValue: "backend\n", wantAnchor: "backend", wantOneRepository: map[string]bool{"worktree": true}},
		{
			name: "EmptyRootMarker_IsRootAnchor", rootValue: "  \n", boardValue: "backend\n",
			wantAnchor: ".", wantOneRepository: map[string]bool{"worktree": true},
		},
		{
			name: "RootMarkerInvalid_IsErrorNamingFile", rootValue: "../out\n",
			wantErr: ErrInvalidAnchor, wantErrFile: AnchorFileName, wantErrAtRoot: true, wantOneRepository: map[string]bool{"worktree": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			board := boardDir(hub)
			ownRoot := weftname.SiblingPath(hub, worktreeName)
			worktreeRoot := filepath.Join(hub, worktreeName)
			for dir, files := range map[string]map[string]string{
				board:        {StaleAnchorFileName: tt.staleValue, AnchorFileName: tt.boardValue},
				ownRoot:      {AnchorFileName: tt.ownValue},
				worktreeRoot: {AnchorFileName: tt.rootValue},
			} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
				for fileName, content := range files {
					if content == "" {
						continue
					}
					if err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o644); err != nil {
						t.Fatalf("write %s: %v", fileName, err)
					}
				}
			}

			for root, dir := range map[string]string{"worktree": worktreeRoot, "sibling": ownRoot, "bolt": board} {
				if got := OneRepository(dir); got != tt.wantOneRepository[root] {
					t.Errorf("OneRepository(%s root) = %v; want %v", root, got, tt.wantOneRepository[root])
				}
			}

			location, err := NamedWorktree(hub, worktreeName)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NamedWorktree() error = %v; want %v", err, tt.wantErr)
				}
				errDir := ownRoot
				if tt.wantErrAtRoot {
					errDir = worktreeRoot
				}
				if tt.wantErrFile != "" && !strings.Contains(err.Error(), filepath.Join(errDir, tt.wantErrFile)) {
					t.Errorf("NamedWorktree() error = %q; want it to name %s", err, filepath.Join(errDir, tt.wantErrFile))
				}
				return
			}
			if err != nil {
				t.Fatalf("NamedWorktree() error = %v; want nil", err)
			}
			want := Location{RepoName: filepath.Base(hub), HubPath: hub, WorktreeName: worktreeName, AnchorRel: tt.wantAnchor}
			if *location != want {
				t.Errorf("NamedWorktree() = %+v; want %+v", *location, want)
			}
		})
	}
}
