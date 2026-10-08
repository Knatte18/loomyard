//go:build tmux

// smoke_switchseed_test.go pins noise class 3's suppression directly:
// TestSmokeSwitchDeclinesStencilSeedPass arranges both stencilstore Warn emitters cmd/lyx's root
// PersistentPreRunE can reach (the dev-refusal warn and the port-back drift warn) and asserts that
// `lyx reed switch`'s stderr is empty.
// The verb runs with a socket path nothing listens on, so it fails fast without touching a tmux server;
// the assertion runs the built binary as a plain subprocess and reads its own stderr stream directly.

package reedcli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// TestSmokeSwitchDeclinesStencilSeedPass is the regression pin: a dev-stamped real binary runs
// `lyx reed switch` against a hub carrying both a stale-but-untouched board stencil (the
// dev-refusal warn's precondition) and a drifted contracts/stencils worktree copy (the port-back
// drift warn's precondition), and stderr must come back empty.
// Assert emptiness only -- never a line count and never a particular message: either emitter alone is
// enough to make stderr non-empty pre-fix, and post-fix stderr is silent because the pass does not run
// at all for an opted-out command.
func TestSmokeSwitchDeclinesStencilSeedPass(t *testing.T) {
	t.Parallel()
	// The dev channel stamp makes stencilstore.ModeFor(buildinfo.IsDev()) return ModeDev;
	// an unstamped binary is production mode and never emits the dev-refusal warn.
	lyxExe := lyxbin.BuildWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())

	registry := stencils.Registry()
	names := registry.Names()
	if len(names) == 0 {
		t.Fatalf("stencils.Registry().Names() returned no names")
	}
	name := names[0]

	// Arrange the dev-refusal warn: a stamp matching its own body plus a body differing from the
	// shipped default is exactly StateUntouched with drift, which reconcileOne warns about and
	// refuses to refresh under ModeDev.
	shipped, known := registry.Default(name)
	if !known {
		t.Fatalf("registry has no default for its own first name %q", name)
	}
	driftedBody := append(append([]byte{}, shipped...), []byte("\nsmoke-drift-line\n")...)
	boardPath := stencilstore.Path(fabricengine.StencilsDir(h.Path), name)
	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
		t.Fatalf("create board stencil parent dir: %v", err)
	}
	stamped := stencilstore.ApplyStamp(driftedBody, stencilstore.BodyHash(driftedBody))
	if err := os.WriteFile(boardPath, stamped, 0o644); err != nil {
		t.Fatalf("write board stencil %s: %v", boardPath, err)
	}

	// Arrange the port-back drift warn: a hubforge fixture worktree carries no contracts/ directory
	// of its own, so seedStencilsAt sets sourceDir empty and warnPortBackDrift cannot fire without
	// this step -- materialize contracts/stencils/<relPath> with a body differing from the board copy
	// just planted.
	sourcePath := filepath.Join(h.PrimeWorktree(), "contracts", "stencils", filepath.FromSlash(stencilstore.RelPath(name)))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create contracts/stencils parent dir: %v", err)
	}
	sourceBody := append(append([]byte{}, shipped...), []byte("\nsmoke-source-drift-line\n")...)
	if err := os.WriteFile(sourcePath, sourceBody, 0o644); err != nil {
		t.Fatalf("write contracts/stencils source %s: %v", sourcePath, err)
	}

	// The verb fails against the dead socket, so only the stderr stream is asserted, never the exit status.
	deadSocket := filepath.Join(t.TempDir(), "no-server")
	cmd := exec.Command(lyxExe, "reed", "switch", "--next", "--socket", deadSocket, "--client", "no-client", "--tmux", "tmux")
	cmd.Dir = h.PrimeWorktree()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("lyx reed switch: %v; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("lyx reed switch stderr = %q; want empty -- switch must decline the root pre-run's stencil-seed pass entirely", stderr.String())
	}
}
