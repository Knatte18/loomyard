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

func TestResolveRunID(t *testing.T) {
	l := syntheticLocation(t)
	if got := ResolveRunID(l, SelfRunID); got != l.WorktreeName {
		t.Errorf("ResolveRunID(self) = %q; want %q", got, l.WorktreeName)
	}
	if got := ResolveRunID(l, "other"); got != "other" {
		t.Errorf("ResolveRunID(other) = %q; want %q", got, "other")
	}
}

func TestResolveRunID_LegacyDirStillAnswersSlug(t *testing.T) {
	l := syntheticLocation(t)
	mkShedDir(t, l, SelfRunID)
	if got := ResolveRunID(l, SelfRunID); got != l.WorktreeName {
		t.Errorf("ResolveRunID(self) with legacy dir = %q; want %q", got, l.WorktreeName)
	}
}

func TestRunDir_SelfAndSlugAddressSlugDirForNewRun(t *testing.T) {
	l := syntheticLocation(t)
	want := filepath.Join(l.AnchorPath(), "_lyx", "shed", l.WorktreeName)
	for _, id := range []string{SelfRunID, l.WorktreeName} {
		if got := RunDir(l, id); got != want {
			t.Errorf("RunDir(%q) = %q; want %q", id, got, want)
		}
	}
}

func TestRunDir_LegacyFallbackWhenOnlySelfExists(t *testing.T) {
	l := syntheticLocation(t)
	mkShedDir(t, l, SelfRunID)
	want := filepath.Join(l.AnchorPath(), "_lyx", "shed", SelfRunID)
	for _, id := range []string{SelfRunID, l.WorktreeName} {
		if got := RunDir(l, id); got != want {
			t.Errorf("RunDir(%q) = %q; want %q", id, got, want)
		}
	}
	wantScratch := filepath.Join(l.AnchorPath(), ".lyx", "shed", SelfRunID)
	if got := ScratchDir(l, SelfRunID); got != wantScratch {
		t.Errorf("ScratchDir(self) = %q; want %q", got, wantScratch)
	}
}

func TestRunDir_SlugDirWinsOverLegacy(t *testing.T) {
	l := syntheticLocation(t)
	mkShedDir(t, l, SelfRunID)
	mkShedDir(t, l, l.WorktreeName)
	want := filepath.Join(l.AnchorPath(), "_lyx", "shed", l.WorktreeName)
	if got := RunDir(l, SelfRunID); got != want {
		t.Errorf("RunDir(self) = %q; want %q", got, want)
	}
}

func TestRunDir_OtherRunIDUnaffectedByLegacy(t *testing.T) {
	l := syntheticLocation(t)
	mkShedDir(t, l, SelfRunID)
	want := filepath.Join(l.AnchorPath(), "_lyx", "shed", "other")
	if got := RunDir(l, "other"); got != want {
		t.Errorf("RunDir(other) = %q; want %q", got, want)
	}
}

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
