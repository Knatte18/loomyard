package shedrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// mkShedDir creates _lyx/shed/<segment> under l's anchor.
func mkShedDir(t *testing.T, l *lyxcwd.Location, segment string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(l.AnchorPath(), "_lyx", "shed", segment), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

//testtiming:keep pins the self-to-worktree-name mapping and the pass-through of another id, which its covering test does not
func TestResolveRunID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		legacyDir bool
		id        string
		want      string
	}{
		{"self resolves to the worktree name", false, SelfRunID, worktreeName},
		{"another id resolves to itself", false, "other", "other"},
		{"a legacy self dir still answers the slug", true, SelfRunID, worktreeName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)
			if tt.legacyDir {
				mkShedDir(t, l, SelfRunID)
			}
			if got := ResolveRunID(l, tt.id); got != tt.want {
				t.Errorf("ResolveRunID(%q) = %q; want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestRunDirResolution pins which directory a run-id addresses: the slug dir for a new run, the legacy self dir while only it exists, the slug dir once both exist, and an unrelated id's own dir.
func TestRunDirResolution(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// existing lists the _lyx/shed segments created before resolving.
		existing []string
		ids      []string
		wantSeg  string
		// wantScratchSelfSeg, when set, is the .lyx/shed segment ScratchDir(self) must resolve to.
		wantScratchSelfSeg string
	}{
		{"a new run is addressed by self or slug at the slug dir", nil, []string{SelfRunID, worktreeName}, worktreeName, ""},
		{"a legacy self dir is the fallback while only it exists", []string{SelfRunID}, []string{SelfRunID, worktreeName}, SelfRunID, SelfRunID},
		{"the slug dir wins over the legacy one", []string{SelfRunID, worktreeName}, []string{SelfRunID}, worktreeName, ""},
		{"another run-id is unaffected by a legacy dir", []string{SelfRunID}, []string{"other"}, "other", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)
			for _, seg := range tt.existing {
				mkShedDir(t, l, seg)
			}
			want := filepath.Join(l.AnchorPath(), "_lyx", "shed", tt.wantSeg)
			for _, id := range tt.ids {
				if got := RunDir(l, id); got != want {
					t.Errorf("RunDir(%q) = %q; want %q", id, got, want)
				}
			}
			if tt.wantScratchSelfSeg != "" {
				wantScratch := filepath.Join(l.AnchorPath(), ".lyx", "shed", tt.wantScratchSelfSeg)
				if got := ScratchDir(l, SelfRunID); got != wantScratch {
					t.Errorf("ScratchDir(self) = %q; want %q", got, wantScratch)
				}
			}
		})
	}
}

//testtiming:keep pins that self resolves against the told location, so a WriteSeed lands at SeedRel and SeedFile, which its covering tests do not
func TestSelfResolvesAgainstTheToldLocation(t *testing.T) {
	a := syntheticLocation(t)
	b := &lyxcwd.Location{RepoName: "repo", HubPath: a.HubPath, WorktreeName: "child-slug", AnchorRel: "."}
	if got := ResolveRunID(b, SelfRunID); got != "child-slug" {
		t.Errorf("ResolveRunID(b, self) = %q; want child-slug", got)
	}
	if err := WriteSeed(b, SelfRunID, Seed{Recipe: RecipeLoom, Driver: DriverGo}); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.AnchorPath(), SeedRel(b, SelfRunID))); err != nil {
		t.Errorf("SeedRel does not name the file WriteSeed wrote: %v", err)
	}
	if _, err := os.Stat(SeedFile(b, "child-slug")); err != nil {
		t.Errorf("SeedFile(child-slug) missing: %v", err)
	}
}

func TestReadWriteSeed_InvalidResolvedIDRefused(t *testing.T) {
	l := syntheticLocation(t)
	if err := WriteSeed(l, "../x", Seed{Recipe: RecipeLoom, Driver: DriverGo}); err == nil {
		t.Error("WriteSeed with an invalid id = nil; want error")
	}
	if _, _, err := ReadSeed(l, "a/b"); err == nil {
		t.Error("ReadSeed with an invalid id = nil; want error")
	}
	empty := &lyxcwd.Location{HubPath: l.HubPath, AnchorRel: "."}
	if err := WriteSeed(empty, SelfRunID, Seed{Recipe: RecipeLoom, Driver: DriverGo}); err == nil {
		t.Error("WriteSeed(self) with an empty worktree name = nil; want error")
	}
}
