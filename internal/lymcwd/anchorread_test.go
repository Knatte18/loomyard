// anchorread_test.go pins the marker semantics of a worktree's recorded anchor in Tier 0.
// Only the worktree's own marker answers, and an absent or empty marker is a fallback.
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
	ownMarker := filepath.Join(weftname.SiblingPath(hub, worktreeName), AnchorFileName)
	if err := os.MkdirAll(ownMarker, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", ownMarker, err)
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
		name string
		// boardValue is the content for AnchorFileName at the Bolt checkout's root, "" for absent.
		// A board marker is never read.
		boardValue string
		// ownValue is the content for AnchorFileName in the worktree's own records sibling, "" for absent.
		ownValue    string
		wantAnchor  string
		wantErr     error
		wantErrFile string // path the error must name, "" for none
	}{
		{name: "AbsentAll_IsFallback", wantAnchor: "."},
		{name: "BoardMarkerOnly_IsIgnored", boardValue: "backend\n", wantAnchor: "."},
		{name: "OwnMarker_WinsOverBoard", boardValue: "backend\n", ownValue: "frontend\n", wantAnchor: "frontend"},
		{name: "OwnMarkerEmpty_IsAbsentNotEmptySubpath", boardValue: "backend\n", ownValue: "  \n", wantAnchor: "."},
		{name: "OwnMarkerOnly", ownValue: "frontend\n", wantAnchor: "frontend"},
		{name: "OwnMarkerInvalid_IsErrorNamingFile", boardValue: "backend\n", ownValue: "../out\n", wantErr: ErrInvalidAnchor, wantErrFile: AnchorFileName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			board := filepath.Join(hub, BoltDirName)
			ownRoot := weftname.SiblingPath(hub, worktreeName)
			for dir, files := range map[string]map[string]string{
				board:   {AnchorFileName: tt.boardValue},
				ownRoot: {AnchorFileName: tt.ownValue},
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

			location, err := NamedWorktree(hub, worktreeName)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NamedWorktree() error = %v; want %v", err, tt.wantErr)
				}
				if tt.wantErrFile != "" && !strings.Contains(err.Error(), filepath.Join(ownRoot, tt.wantErrFile)) {
					t.Errorf("NamedWorktree() error = %q; want it to name %s", err, filepath.Join(ownRoot, tt.wantErrFile))
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
