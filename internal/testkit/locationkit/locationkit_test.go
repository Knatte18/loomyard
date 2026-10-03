package locationkit

import (
	"path/filepath"
	"testing"
)

func TestLocation_CarriesToldFields(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-LYXHUB")
	l := Location(hub, "slug", "sub/dir")

	if l.HubPath != hub || l.WorktreeName != "slug" || l.AnchorRel != "sub/dir" {
		t.Fatalf("Location = %+v; want HubPath %q, WorktreeName %q, AnchorRel %q", l, hub, "slug", "sub/dir")
	}
	if l.RepoName != "" {
		t.Fatalf("RepoName = %q; want empty", l.RepoName)
	}
}

func TestLocation_PathsJoinFields(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-LYXHUB")
	l := Location(hub, "slug", "sub")

	wantWorktree := filepath.Join(hub, "slug")
	if got := l.WorktreePath(); got != wantWorktree {
		t.Fatalf("WorktreePath() = %q; want %q", got, wantWorktree)
	}
	wantAnchor := filepath.Join(wantWorktree, "sub")
	if got := l.AnchorPath(); got != wantAnchor {
		t.Fatalf("AnchorPath() = %q; want %q", got, wantAnchor)
	}
}
