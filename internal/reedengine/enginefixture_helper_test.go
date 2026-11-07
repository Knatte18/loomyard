// enginefixture_helper_test.go holds the engine fixture and the helpers that the tier0 and tier2 tests share.
// It carries no build tag, so it compiles into every tag set.

package reedengine

import (
	"os"
	"path/filepath"
	"testing"
)

// testScrub is Claude's declared scrub, spelled out because this package's tests cannot import the registry that owns it.
var testScrub = EnvScrub{Names: []string{"CLAUDECODE"}, Prefixes: []string{"CLAUDE_CODE_"}}

// newTestEngine builds an Engine rooted at a fresh t.TempDir().
// It suits a test that needs an *Engine but must never shell out to tmux.
// The Geometry is a struct literal because hubgeom imports reedengine,
// so an in-package test importing it would close an import cycle.
// cfg.Tmux and cfg.Shell point at paths that do not exist.
// A stray real invocation therefore fails fast with "file not found".
// It never hangs or silently succeeds against an unrelated running server.
// The Geometry fields are distinct paths under one t.TempDir().
// They are a synthetic hub, a worktree root under it, and an anchor path under that.
// A field mix-up inside the engine therefore surfaces instead of passing silently.
// Only the worktree root is created on disk.
// AnchorPath and PaneCwd stay uncreated, so a field mix-up between them and WorktreeRoot surfaces.
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
	// PaneCwd is set to a directory distinct from AnchorPath,
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
		// A valid value: a caller reaching ensureServerAndSessionLocked then passes watchdogOption's boot-path validation.
		// A test that wants an invalid value overrides e.cfg.Watchdog itself.
		Watchdog: "on",
		// Only the segment keys set here get a color; any other segment key resolves to no color.
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
