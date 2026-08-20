//go:build scout

// refs_integration_test.go exercises References against a real, held-open
// gopls subprocess — the one test in this package that actually launches a
// language server. It is //go:build scout-tagged and therefore
// excluded from the plain `go test` verify (the Test Tier Purity
// Invariant); it is run separately with `-tags scout` on a machine
// with gopls installed. Only the gopls-spawning subtest is guarded on
// exec.LookPath("gopls") (via t.Skip); the ErrServerNotFound subtest never
// launches gopls and always runs, even on a machine without it. This test
// only spawns gopls, never git, so no TestMain/gitkit.HermeticGitEnv is
// required per the Hermetic Git Test Environment Invariant.

package scoutengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// funcDeclPattern matches top-level function declarations and captures the function name offset.
var funcDeclPattern = regexp.MustCompile(`^func (?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\(`)

// findFuncPosition returns the Position of a top-level function in a file.
func findFuncPosition(t *testing.T, file, funcName string) Position {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("findFuncPosition: read %s: %v", file, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		m := funcDeclPattern.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		name := line[m[2]:m[3]]
		if name != funcName {
			continue
		}
		return Position{File: file, Line: i + 1, Character: m[2] + 1}
	}
	t.Fatalf("findFuncPosition: no top-level declaration of %q found in %s", funcName, file)
	return Position{}
}

// repoRoot returns this worktree's module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRoot: could not determine scoutengine source directory location")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func TestReferences_Integration(t *testing.T) {
	t.Run("live gopls references for a known high-fan-in symbol", func(t *testing.T) {
		if _, err := exec.LookPath("gopls"); err != nil {
			t.Skip(builtins()["go"].InstallHint)
		}

		root := repoRoot(t)
		lyxcwdFile := filepath.Join(root, "internal", "lyxcwd", "lyxcwd.go")
		pos := findFuncPosition(t, lyxcwdFile, "Resolve")

		// Since the engine-supervised-flip batch, Go's registry entry
		// dispatches through ensureServer -> ensureSupervised, which spawns a
		// lyx-owned daemon that teardownConnection's connKindSupervised
		// branch deliberately never kills. Without an explicit
		// anchorRoot this anchors at a relative .lyx/scout/go/ under the
		// test binary's cwd and leaks the daemon; an isolated t.TempDir()
		// plus a state-file-driven reap avoids both.
		worktreeRoot := t.TempDir()
		statePath := DaemonStateFile(worktreeRoot, "go")
		t.Cleanup(func() { killRecordedDaemon(t, statePath) })

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		refs, err := References(ctx, Options{
			Registry:   builtins(),
			TargetDir:  root,
			AnchorRoot: worktreeRoot,
			Lang:       "go",
			Query:      Query{Pos: &pos},
			Timeout:    30 * time.Second,
		})
		if err != nil {
			t.Fatalf("References(lyxcwd.Resolve) returned unexpected error: %v", err)
		}
		if len(refs) == 0 {
			t.Fatal("References(lyxcwd.Resolve) returned zero references; want the declaration site plus its call sites")
		}

		foundDeclSite := false
		for _, ref := range refs {
			if filepath.Clean(ref.File) == filepath.Clean(lyxcwdFile) && ref.Line == pos.Line {
				foundDeclSite = true
				break
			}
		}
		if !foundDeclSite {
			t.Errorf("References(lyxcwd.Resolve) = %+v; want it to include the declaration site %s:%d", refs, lyxcwdFile, pos.Line)
		}
	})

	t.Run("non-existent server binary yields ErrServerNotFound", func(t *testing.T) {
		root := repoRoot(t)
		reg := Registry{
			"go": {
				Markers:     []string{"go.mod"},
				Match:       "any",
				Command:     []string{"lyx-scout-nonexistent-binary-xyz"},
				InstallHint: "this binary is intentionally fake for the test",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := References(ctx, Options{
			Registry:  reg,
			TargetDir: root,
			Lang:      "go",
			Query:     Query{Symbol: "Resolve"},
			Timeout:   5 * time.Second,
		})
		if !errors.Is(err, ErrServerNotFoundSentinel) {
			t.Errorf("References() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
		}
	})
}

// writeAmbiguousModule writes a minimal, self-contained Go module to dir: a
// go.mod plus one .go file declaring the same method name ("Open") on two
// distinct types. This is what makes a name-only, per-file
// textDocument/documentSymbol lookup for "Open" within that file resolve to
// more than one candidate — the case that distinguishes InFile's exhaustive
// per-file search from a single hit, exercised by
// TestReferences_InFile_Integration's "same-name-in-two-types ambiguity"
// subtest.
func writeAmbiguousModule(t *testing.T, dir string) {
	t.Helper()
	goMod := "module ambiguousmodule\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("os.WriteFile(go.mod) failed: %v", err)
	}
	src := `package main

type FileHandle struct{}

func (h FileHandle) Open() error { return nil }

type SocketHandle struct{}

func (h SocketHandle) Open() error { return nil }

func main() {}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("os.WriteFile(main.go) failed: %v", err)
	}
}

// TestReferences_InFile_Integration proves the Query.InFile resolve path — documentSymbol ->
// position -> textDocument/references — end to end against a real gopls, the InFile analogue of
// TestReferences_Integration's Query.Pos coverage above.
// Both subcases route through ensureServer's now- live supervised dispatch (builtins()'s Go entry
// has HasNativeDaemon: true), which spawns a lyx-owned daemon that teardownConnection's
// connKindSupervised branch deliberately never kills — each subcase anchors its anchorRoot at its
// own isolated t.TempDir() and reaps the spawned daemon in t.Cleanup, exactly like
// TestEnsureServer_Integration_ SupervisedDispatch (ensureserver_integration_test.go) and
// supervised_integration_test.go already do.
func TestReferences_InFile_Integration(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	t.Run("single-match resolve", func(t *testing.T) {
		root := repoRoot(t)
		lyxcwdFile := filepath.Join(root, "internal", "lyxcwd", "lyxcwd.go")
		pos := findFuncPosition(t, lyxcwdFile, "Resolve")

		// TargetDir stays the real repo root (correct indexing), but
		// anchorRoot is an isolated temp dir so the supervised daemon this
		// call spawns anchors there, never the real repo's own
		// .lyx/scout/go/.
		worktreeRoot := t.TempDir()
		statePath := DaemonStateFile(worktreeRoot, "go")
		t.Cleanup(func() { killRecordedDaemon(t, statePath) })

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		refs, err := References(ctx, Options{
			Registry:   builtins(),
			TargetDir:  root,
			AnchorRoot: worktreeRoot,
			Lang:       "go",
			Query:      Query{InFile: &InFileQuery{File: lyxcwdFile, Name: "Resolve"}},
			Timeout:    30 * time.Second,
		})
		if err != nil {
			t.Fatalf("References(InFile lyxcwd.Resolve) returned unexpected error: %v", err)
		}
		if len(refs) == 0 {
			t.Fatal("References(InFile lyxcwd.Resolve) returned zero references; want the declaration site plus its call sites")
		}

		foundDeclSite := false
		for _, ref := range refs {
			if filepath.Clean(ref.File) == filepath.Clean(lyxcwdFile) && ref.Line == pos.Line {
				foundDeclSite = true
				break
			}
		}
		if !foundDeclSite {
			t.Errorf("References(InFile lyxcwd.Resolve) = %+v; want it to include the declaration site %s:%d", refs, lyxcwdFile, pos.Line)
		}
	})

	t.Run("same-name-in-two-types ambiguity", func(t *testing.T) {
		modRoot := t.TempDir()
		writeAmbiguousModule(t, modRoot)

		worktreeRoot := t.TempDir()
		statePath := DaemonStateFile(worktreeRoot, "go")
		t.Cleanup(func() { killRecordedDaemon(t, statePath) })

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		_, err := References(ctx, Options{
			Registry:   builtins(),
			TargetDir:  modRoot,
			AnchorRoot: worktreeRoot,
			Lang:       "go",
			Query:      Query{InFile: &InFileQuery{File: filepath.Join(modRoot, "main.go"), Name: "Open"}},
			Timeout:    30 * time.Second,
		})
		if !errors.Is(err, ErrAmbiguousSymbolSentinel) {
			t.Errorf("References(InFile Open, two types) err = %v; want errors.Is(err, ErrAmbiguousSymbolSentinel)", err)
		}
	})
}
