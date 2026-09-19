// testsupport_test.go builds the shared fake this whole package's test suite drives: a
// *shedengine.Shed over Stub rows (or a controllable funcProducer row for the cases a Stub cannot
// drive) anchored under a t.TempDir(), plus a helper that executes one built command through
// clihelp and decodes the emitted JSON envelope.
//
// No test anywhere in this package constructs a real hub, spawns a process, or touches an LLM,
// tmux, or git -- every fake here is tier 1.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// testPaths bundles one fake shed's three told paths, anchored under a single t.TempDir().
type testPaths struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
}

// newTestPaths allocates a fresh testPaths anchored under a fresh t.TempDir().
func newTestPaths(t *testing.T) testPaths {
	t.Helper()
	dir := t.TempDir()
	return testPaths{
		StatusPath:     filepath.Join(dir, "status.json"),
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: filepath.Join(dir, "status.lock"),
	}
}

// seedStatus writes an initial running status file at paths.StatusPath naming current as
// current_producer, exactly as a real bootstrap would before the generic run/step bodies' first
// call -- shedengine.Shed never seeds a status file itself, so every test driving Run or Step must
// seed one first.
func seedStatus(t *testing.T, paths testPaths, current string) {
	t.Helper()
	if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, shedengine.Status{
		CurrentProducer: current,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
}

// stubRow returns a ProducerDef over loomshed.NewStub(name), terminal (empty OnDone): the fake
// shed's ordinary, no-LLM, no-tmux, no-git happy-path row.
func stubRow(name string) shedengine.ProducerDef {
	return shedengine.ProducerDef{Name: name, Producer: loomshed.NewStub(name)}
}

// funcProducer is a controllable fake ShedProducer for the cases a plain Stub row cannot drive: a
// producer hard error or a Stuck outcome.
type funcProducer struct {
	call func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)
}

// Call implements shedengine.ShedProducer by delegating to the injected closure.
func (p *funcProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return p.call(ctx)
}

// erroringRow returns a ProducerDef named name whose Call always returns callErr as a hard error.
func erroringRow(name string, callErr error) shedengine.ProducerDef {
	return shedengine.ProducerDef{Name: name, Producer: &funcProducer{
		call: func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
			return "", shedengine.OutputPointer{}, callErr
		},
	}}
}

// stuckRow returns a ProducerDef named name whose Call always reports Stuck, routed via onStuck
// (empty means escalate with no target).
func stuckRow(name, onStuck string) shedengine.ProducerDef {
	return shedengine.ProducerDef{
		Name:    name,
		OnStuck: onStuck,
		Producer: &funcProducer{
			call: func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
				return shedengine.Stuck, shedengine.OutputPointer{}, nil
			},
		},
	}
}

// newFakeShed builds a *shedengine.Shed over producers and paths' three told fields, with no
// CommitStatus -- the tier-1 fake every test in this package drives instead of a real hub.
func newFakeShed(paths testPaths, producers []shedengine.ProducerDef) *shedengine.Shed {
	return &shedengine.Shed{
		Producers:      producers,
		StatusPath:     paths.StatusPath,
		LockPath:       paths.LockPath,
		StatusLockPath: paths.StatusLockPath,
	}
}

// countingBuildShed wraps build in a call counter so a test can assert BuildShed was never called
// on a path that must short-circuit before reaching it (a PreRun or PreStep error).
func countingBuildShed(build func() (*shedengine.Shed, error)) (func() (*shedengine.Shed, error), *int) {
	calls := 0
	wrapped := func() (*shedengine.Shed, error) {
		calls++
		return build()
	}
	return wrapped, &calls
}

// execEnvelope runs cmd through clihelp.Execute against a fresh buffer and decodes the LAST
// non-empty printed line as the envelope JSON, tolerating any extra lines a test's own hooks or a
// --watch poll might have printed before the final envelope.
func execEnvelope(t *testing.T, cmd *cobra.Command, args []string) (map[string]any, int) {
	t.Helper()
	var buf bytes.Buffer
	code := clihelp.Execute(cmd, &buf, args)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	last := lines[len(lines)-1]
	var env map[string]any
	if err := json.Unmarshal([]byte(last), &env); err != nil {
		t.Fatalf("decode envelope from %q: %v", buf.String(), err)
	}
	return env, code
}
