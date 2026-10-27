//go:build tier1

// dobbytrace_integration_test.go pins that each exit `lym beam dobby` takes before dobbyengine.Run leaves a line in the dobby's log and, after a re-exec, a notice, over a hubforge hub.

package beamcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// postedNotice is one call of the notify seam.
type postedNotice struct {
	worktree, slug, program, line string
}

func TestRunDobbyWith_TracesEachExitBeforeRun(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location
	primeGeom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry(prime) = %v; want nil", err)
	}
	if err := os.MkdirAll(filepath.Dir(primeGeom.LogPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	task, err := lymcwd.NamedWorktree(h.Path, "task1")
	if err != nil {
		t.Fatalf("NamedWorktree(task1) = %v; want nil", err)
	}

	var notices []postedNotice
	notify := func(from *lymcwd.Location, slug, program, line string) (bool, error) {
		notices = append(notices, postedNotice{from.WorktreeName, slug, program, line})
		return true, nil
	}
	failWiring := func(*lymcwd.Location, dobbyengine.Geometry) (dobbyengine.Deps, error) {
		return dobbyengine.Deps{}, errors.New("hub unreadable")
	}
	run := func(root string, wire func(*lymcwd.Location, dobbyengine.Geometry) (dobbyengine.Deps, error)) string {
		t.Helper()
		var out bytes.Buffer
		runDobbyWith(context.Background(), &out, root, wire, notify)
		return out.String()
	}
	readLog := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) = %v; want nil", path, err)
		}
		return string(data)
	}
	recordThisPid := func(statePath string) {
		t.Helper()
		if err := dobbyengine.UpdateState(statePath, func(s *dobbyengine.State) error { s.PID = os.Getpid(); return nil }); err != nil {
			t.Fatalf("UpdateState(%s) = %v; want nil", statePath, err)
		}
	}

	// The rows run in sequence over one hub, the last removing the prime's worktree link.
	const startFailed = "start failed: wire the dobby over the hub: hub unreadable"
	const wayForward = "fix the cause, then run lym beam start"

	envelope.RequireErr(t, run(h.PrimeWorktree(), failWiring), "hub unreadable")
	if got := readLog(primeGeom.LogPath); !strings.Contains(got, startFailed) || len(notices) != 0 {
		t.Fatalf("prime log, notices after a failed wiring with no state record = %q, %v; want the start-failed line and no notice", got, notices)
	}

	recordThisPid(primeGeom.StatePath)
	envelope.RequireErr(t, run(h.PrimeWorktree(), failWiring), "hub unreadable")
	if len(notices) != 1 {
		t.Fatalf("notices after a failed wiring with this pid recorded = %v; want one", notices)
	}
	if n := notices[0]; n.worktree != prime.WorktreeName || n.slug != "" || n.program != dobbyProgram ||
		!strings.Contains(n.line, "hub unreadable") || !strings.Contains(n.line, wayForward) {
		t.Errorf("notice = %+v; want the prime's, unscoped, signed by the dobby, naming the wiring error and %q", n, wayForward)
	}

	taskBeamDir := hubgeom.DobbyBeamDir(task)
	if err := os.MkdirAll(taskBeamDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	recordThisPid(filepath.Join(taskBeamDir, dobbyengine.StateFileName))
	envelope.RequireErr(t, run(h.PairCodeWorktree("task1"), failWiring), "is neither the prime nor a beam")
	if got := readLog(filepath.Join(taskBeamDir, dobbyengine.LogFileName)); !strings.Contains(got, "start refused: lym beam dobby: task1 is neither the prime nor a beam") {
		t.Errorf("task pair log = %q; want the start-refused line", got)
	}
	if len(notices) != 2 {
		t.Fatalf("notices after the refusal = %v; want a second", notices)
	}
	if n := notices[1]; n.worktree != "task1" || n.slug != "task1" || !strings.Contains(n.line, "is neither the prime nor a beam") || !strings.Contains(n.line, wayForward) {
		t.Errorf("notice = %+v; want task1's, naming the refusal and %q", n, wayForward)
	}

	envelope.RequireErr(t, run(h.PrimeWorktree(), func(*lymcwd.Location, dobbyengine.Geometry) (dobbyengine.Deps, error) {
		if err := os.RemoveAll(filepath.Join(h.PrimeWorktree(), ".git")); err != nil {
			t.Fatalf("RemoveAll(.git) = %v; want nil", err)
		}
		return dobbyengine.Deps{}, errors.New("hub unreadable")
	}), "hub unreadable")
	if got := strings.Count(readLog(primeGeom.LogPath), startFailed); got != 3 {
		t.Errorf("start-failed lines in the prime log = %d; want 3, the last written although the worktree no longer resolves", got)
	}
	if len(notices) != 2 {
		t.Errorf("notices after the unresolvable worktree = %v; want still two", notices)
	}
}
