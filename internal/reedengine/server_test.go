// server_test.go verifies ServerName's determinism, socket-safety, and per-hub uniqueness, plus
// SessionName's worktree-slug derivation.
// ServerName is the SINGLE derivation of the -L socket key: hubgeom.ReedGeometry calls it to fill
// Geometry.SocketKey, and Engine.Socket returns that field verbatim, so there is no second
// spelling here to cross-check it against.

package reedengine

import (
	"path/filepath"
	"regexp"
	"testing"
)

// socketUnsafeChars matches the characters ServerName must never
// produce: ':', '\', and space, all of which are unsafe in a tmux -L
// socket argument.
var socketUnsafeChars = regexp.MustCompile(`[:\\ ]`)

func TestServerName_Deterministic(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got1 := ServerName(hub)
	got2 := ServerName(hub)
	if got1 != got2 {
		t.Errorf("ServerName not deterministic: %q != %q", got1, got2)
	}
}

func TestServerName_SocketSafe(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got := ServerName(hub)
	if socketUnsafeChars.MatchString(got) {
		t.Errorf("ServerName(%q) = %q contains a socket-unsafe character", hub, got)
	}
}

func TestServerName_DistinctForDistinctHubsSharingBasename(t *testing.T) {
	base := "loomyard-HUB"
	hubA := filepath.Join(t.TempDir(), "a", base)
	hubB := filepath.Join(t.TempDir(), "b", base)

	got1 := ServerName(hubA)
	got2 := ServerName(hubB)
	if got1 == got2 {
		t.Errorf("ServerName collided for distinct hubs sharing a basename: %q == %q (hubA=%q, hubB=%q)", got1, got2, hubA, hubB)
	}
}

func TestServerName_HasHubBasenameAndPrefix(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "loomyard-HUB")
	got := ServerName(hub)
	want := "lyx-loomyard-HUB-"
	if len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("ServerName(%q) = %q, want prefix %q", hub, got, want)
	}
	// Everything after the prefix must be exactly 8 lowercase hex chars.
	hash := got[len(want):]
	if len(hash) != 8 {
		t.Errorf("ServerName(%q) hash suffix = %q, want length 8", hub, hash)
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("ServerName(%q) hash suffix %q has non-hex char %c", hub, hash, c)
		}
	}
}

func TestSessionName_IsWorktreeBasename(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "internal-reed")
	got := SessionName(worktree)
	want := "internal-reed"
	if got != want {
		t.Errorf("SessionName(%q) = %q, want %q", worktree, got, want)
	}
}
