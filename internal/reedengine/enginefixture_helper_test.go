// enginefixture_helper_test.go holds the engine fixture and small helpers the tier0 and tier2 tests of this package share.
// newTestEngine builds a Geometry struct literal directly rather than calling hubgeom.ReedGeometry.
// hubgeom imports reedengine, so an in-package test importing it would close an import cycle.
// It carries no build tag, so it compiles into every tag set.

package reedengine

import (
	"os"
	"path/filepath"
	"testing"
)

// testScrub is Claude's declared scrub, spelled out because this package's tests cannot import the registry that owns it.
var testScrub = EnvScrub{Names: []string{"CLAUDECODE"}, Prefixes: []string{"CLAUDE_CODE_"}}

// newTestEngine builds an Engine rooted at a fresh t.TempDir(), suitable for any reedengine test that needs a *Engine but must never actually shell out to tmux.
// cfg.Tmux and cfg.Shell point at paths that do not exist,
// so a stray real invocation fails fast with "file not found" instead of hanging or silently succeeding against some unrelated running server.
// The Geometry fields are distinct values derived from one t.TempDir(): a synthetic hub, a worktree root under it, and an anchor path under that.
// A field mix-up inside the engine therefore surfaces instead of passing silently.
// Only the worktree root is created on disk.
// AnchorPath and PaneCwd are deliberately left uncreated, which makes a field mix-up between them and WorktreeRoot surface.
// It also makes this shared fixture stand in for the standalone shape.
// There the anchor is a state directory the engine itself materializes on first use.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	hub := t.TempDir()
	worktreeRoot := filepath.Join(hub, "worktree")
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll worktree root: %v", err)
	}
	anchorPath := filepath.Join(worktreeRoot, "anchor")
	// PaneCwd is deliberately set to a directory distinct from AnchorPath,
	// so a spawn site that regresses to reading AnchorPath instead of PaneCwd cannot pass by coincidence.
	geom := Geometry{
		SocketKey:     ServerName(hub),
		SessionName:   SessionName(worktreeRoot),
		AnchorPath:    anchorPath,
		PaneCwd:       filepath.Join(hub, "pane"),
		WorktreeRoot:  worktreeRoot,
		LogsDir:       filepath.Join(hub, "logs"),
		HubPath:       hub,
		NameShortname: "tc",
		NameSlug:      "tslug",
	}
	cfg := Config{
		Tmux:   filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell:  filepath.Join(hub, "does-not-exist-shell.exe"),
		Width:  100,
		Height: 21,
		// A valid value, so a caller reaching ensureServerAndSessionLocked does not fail on watchdogOption's boot-path validation.
		// A test that wants an invalid value overrides e.cfg.Watchdog itself.
		Watchdog: "on",
		// Two configured segments; any other segment key resolves to no color.
		SegmentColors: map[string]string{"review": "orange", "plan": "cyan"},
	}
	return New(cfg, geom, testScrub)
}

// containsArg reports whether want appears verbatim anywhere in args.
func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
