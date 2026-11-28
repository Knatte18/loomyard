// smoke_switchseed_test.go pins that `lym reed switch` writes nothing to stderr.
// The test arms both stencilstore Warn emitters that cmd/lym's root PersistentPreRunE can reach.
// They are the dev-refusal warn and the port-back drift warn.
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
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSmokeSwitchDeclinesStencilSeedPass runs `lym reed switch` from a dev-stamped binary.
// The hub holds an untouched board stencil that differs from the shipped default,
// and the worktree's contracts/stencils copy has drifted.
// The test asserts emptiness only, never a line count or a particular message.
// The seed pass does not run at all for a command that opts out, so stderr stays silent.
func TestSmokeSwitchDeclinesStencilSeedPass(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	// The dev channel stamp makes stencilstore.ModeFor return ModeDev.
	// ModeDev warns instead of refreshing an untouched stencil.
	lymExe := lymbin.BuildWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())

	registry := stencils.Registry()
	names := registry.Names()
	if len(names) == 0 {
		t.Fatalf("stencils.Registry().Names() returned no names")
	}
	name := names[0]

	// Arrange the dev-refusal warn: a stamp matching its body, and a body differing from the shipped default.
	// That is StateUntouched with drift.
	// reconcileOne warns about it and refuses to refresh it under ModeDev.
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

	// Arrange the port-back drift warn: a hubforge fixture worktree has no contracts/ directory of its own.
	// seedStencilsAt therefore sets sourceDir empty.
	// Without this step, warnPortBackDrift cannot fire.
	// Materialize contracts/stencils/<relPath> with a body differing from the board copy planted above.
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
	cmd := exec.Command(lymExe, "reed", "switch", "--next", "--socket", deadSocket, "--client", "no-client", "--tmux", "tmux")
	cmd.Dir = h.PrimeWorktree()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("lym reed switch: %v; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("lym reed switch stderr = %q; want empty -- switch must decline the root pre-run's stencil-seed pass entirely", stderr.String())
	}
}
