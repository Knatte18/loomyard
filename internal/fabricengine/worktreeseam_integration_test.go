// worktreeseam_integration_test.go checks where fabric fires its worktree seam.
// It also checks that a seam failure is a warning and never fails a verb.
// It installs the process-wide seam, so it runs without t.Parallel;
// the recording fake reports only the events under the hub a subtest asks about.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The two kinds of worktree event.
const (
	seamMade   = "made"
	seamTookIn = "took in"
)

// seamHeal is the way forward every seam warning names.
const seamHeal = "lym heddle render"

// recordedEvent is one event the fake seam heard, with what held true when it fired.
type recordedEvent struct {
	kind  string
	event fabricengine.WorktreeEvent
	// boltPresent is whether the Bolt checkout existed when the seam was called.
	boltPresent bool
	// linesSeeded is whether every excluded path was in the repository's exclude file when the seam was called.
	linesSeeded bool
}

// fakeWorktreeSeam records every event it hears and fails the ones under a directory it was told to fail.
type fakeWorktreeSeam struct {
	t         *testing.T
	mu        sync.Mutex
	events    []recordedEvent
	failUnder string
}

func (s *fakeWorktreeSeam) Made(event fabricengine.WorktreeEvent) error {
	return s.hear(seamMade, event)
}

func (s *fakeWorktreeSeam) TookIn(event fabricengine.WorktreeEvent) error {
	return s.hear(seamTookIn, event)
}

func (s *fakeWorktreeSeam) hear(kind string, event fabricengine.WorktreeEvent) error {
	_, boltErr := os.Stat(event.BoltDir)
	recorded := recordedEvent{kind: kind, event: event, boltPresent: boltErr == nil, linesSeeded: excludeLinesPresent(s.t, event)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, recorded)
	if s.failUnder != "" && within(event.Root, s.failUnder) {
		return errors.New("the fake seam refuses")
	}
	return nil
}

// failWithin makes the seam fail every event under dir; an empty dir heals it.
func (s *fakeWorktreeSeam) failWithin(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failUnder = dir
}

// under returns the events of kind whose root lies under dir, in the order they fired.
func (s *fakeWorktreeSeam) under(dir, kind string) []recordedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found []recordedEvent
	for _, recorded := range s.events {
		if recorded.kind == kind && within(recorded.event.Root, dir) {
			found = append(found, recorded)
		}
	}
	return found
}

// within reports whether path is dir or lies under it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// excludeLinesPresent reports whether every path the event excludes has its anchored line in the exclude file.
// The exclude file is the one of the repository checked out at the event's root.
func excludeLinesPresent(t *testing.T, event fabricengine.WorktreeEvent) bool {
	t.Helper()
	commonDir := strings.TrimSpace(gitkit.Git(t, event.Root, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(event.Root, commonDir)
	}
	content, err := os.ReadFile(filepath.Join(commonDir, "info", "exclude"))
	if err != nil {
		return false
	}
	lines := strings.Split(string(content), "\n")
	for _, path := range event.Excluded {
		if !slices.Contains(lines, "/"+path) {
			return false
		}
	}
	return true
}

// requireOneEvent fails the test unless exactly one event of kind lies under dir, and returns it.
func requireOneEvent(t *testing.T, seam *fakeWorktreeSeam, dir, kind string) recordedEvent {
	t.Helper()
	found := seam.under(dir, kind)
	if len(found) != 1 {
		t.Fatalf("%q events under %s = %d; want 1", kind, dir, len(found))
	}
	return found[0]
}

// requireWarning fails the test unless warning names worktree and the render verb that repairs it.
func requireWarning(t *testing.T, label, warning, worktree string) {
	t.Helper()
	if !strings.Contains(warning, worktree) || !strings.Contains(warning, seamHeal) {
		t.Errorf("%s = %q; want it to name %s and %q", label, warning, worktree, seamHeal)
	}
}

// TestWorktreeSeam_Events covers when fabric fires its worktree seam and what the events hold.
// It also covers that a failing seam is a warning and never a failed verb.
// It runs without t.Parallel because it installs the process-wide seam and captures the logger.
func TestWorktreeSeam_Events(t *testing.T) {
	tiergate.Require(t, 1)
	logs := logcapture.Capture(t)

	t.Run("With no seam installed a pull that fast-forwards warp carries no warning", func(t *testing.T) {
		hub := newOriginHub(t)
		originCommitFile(t, hub.CodeBare, hub.primeBranch, "AGENTS.md", "moved")

		result, err := hubforge.OpenFabric(t, hub.Hub).Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		if !result.WarpAdvanced || result.SeamWarning != "" {
			t.Errorf("Pull() WarpAdvanced = %v, SeamWarning = %q; want warp advanced with no warning", result.WarpAdvanced, result.SeamWarning)
		}
	})

	seam := &fakeWorktreeSeam{t: t}
	t.Cleanup(fabricengine.InstallWorktreeSeam(seam))

	hub := hubforge.NewHub(t, ".")
	second := hubforge.CloneSecond(t, hub)
	fab := hubforge.OpenFabric(t, hub)

	t.Run("A hub made through the clone and a pair added to it each fire one made event", func(t *testing.T) {
		made := requireOneEvent(t, seam, hub.Path, seamMade)
		if made.event.Root != hub.PrimeWorktree() {
			t.Errorf("clone made event root = %q; want the prime worktree %q", made.event.Root, hub.PrimeWorktree())
		}
		if !made.boltPresent || !made.linesSeeded || len(made.event.Excluded) == 0 || len(made.event.Paths) != 0 {
			t.Errorf("clone made event = %+v; want the Bolt checkout present, the exclude lines seeded, a non-empty exclude list and no paths", made)
		}

		hubforge.AddPair(t, hub, "p1")
		pair := seam.under(hub.Path, seamMade)
		if len(pair) != 2 {
			t.Fatalf("made events after the add = %d; want 2", len(pair))
		}
		added := pair[1]
		wantScratch := filepath.Join(hub.PairRecordsSibling("p1"), lymdirs.DotLymDirName)
		if added.event.Root != hub.PairCodeWorktree("p1") || added.event.ScratchDir != wantScratch || added.event.BoltDir != hub.BoardDir() {
			t.Errorf("added made event = %+v; want root %q, scratch dir %q and Bolt dir %q", added.event, hub.PairCodeWorktree("p1"), wantScratch, hub.BoardDir())
		}
		if !added.boltPresent || !added.linesSeeded {
			t.Errorf("added made event = %+v; want the Bolt checkout present and the exclude lines seeded", added)
		}
	})

	t.Run("A failing seam leaves the added pair standing and logs a warning naming it", func(t *testing.T) {
		seam.failWithin(hub.Path)
		t.Cleanup(func() { seam.failWithin("") })
		logs.Reset()

		result := hubforge.AddPair(t, hub, "p2")
		if _, err := os.Stat(result.Path); err != nil {
			t.Errorf("added pair %s is missing: %v; want it standing despite the seam failure", result.Path, err)
		}
		requireWarning(t, "log after the add", logs.String(), hub.PairCodeWorktree("p2"))
	})

	t.Run("A pull that fast-forwards warp fires one took-in event over the changed paths", func(t *testing.T) {
		for _, rel := range []string{"AGENTS.md", "src/x.go"} {
			before := len(seam.under(hub.Path, seamTookIn))
			moveOrigin(t, second.PrimeWorktree(), rel)

			result, err := fab.Pull(fabricengine.SyncOptions{})
			if err != nil {
				t.Fatalf("Pull() over %s error = %v", rel, err)
			}
			events := seam.under(hub.Path, seamTookIn)
			if len(events) != before+1 || !slices.Equal(events[before].event.Paths, []string{rel}) || events[before].event.Root != hub.PrimeWorktree() {
				t.Fatalf("took-in events after pulling %s = %+v; want one more, at the prime worktree, over exactly [%s]", rel, events, rel)
			}
			if result.SeamWarning != "" {
				t.Errorf("Pull() SeamWarning = %q; want none", result.SeamWarning)
			}
		}
	})

	t.Run("A pull over rewritten warp history fires one took-in event over the paths between the old and new head", func(t *testing.T) {
		fixturesDir := t.TempDir()
		f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 3)
		oldHead := warpSHAs[2]
		newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[1])

		if _, err := f.Pull(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		want := strings.Fields(gitkit.Git(t, warpPath, "diff", "--name-only", "--no-renames", oldHead, newTip))
		slices.Sort(want)
		got := requireOneEvent(t, seam, weftFixture.Path, seamTookIn).event.Paths
		slices.Sort(got)
		if !slices.Equal(got, want) || len(want) == 0 {
			t.Errorf("took-in paths after the rewrite = %v; want the paths between the old and new head %v", got, want)
		}
	})

	t.Run("A Bolt pull and a weft-only fast-forward fire nothing", func(t *testing.T) {
		origin := newOriginHub(t)
		originCommitFile(t, origin.RecordsBare, "main", "moved.txt", "moved")
		originCommitFile(t, origin.RecordsBare, origin.primeRecords, "moved.txt", "moved")

		if _, _, err := fabricengine.NewBolt(origin.BoardDir()).Pull(fabricengine.SyncOptions{}, fabricengine.NewMutations(origin.Path)); err != nil {
			t.Fatalf("Bolt Pull() error = %v", err)
		}
		result, err := hubforge.OpenFabric(t, origin.Hub).Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		if !result.WeftPulled || result.WarpAdvanced {
			t.Fatalf("Pull() WeftPulled = %v, WarpAdvanced = %v; want only the weft moved", result.WeftPulled, result.WarpAdvanced)
		}
		if events := append(seam.under(origin.Path, seamTookIn), seam.under(origin.Path, seamMade)...); len(events) != 0 {
			t.Errorf("events under the hub = %+v; want none", events)
		}
	})

	t.Run("A failing seam never fails a pull, and its warning reaches the result, the log, the verb's output and the hub pull's line", func(t *testing.T) {
		origin := newOriginHub(t)
		prime := locationOf(t, origin.PrimeWorktree())
		seam.failWithin(origin.Path)
		t.Cleanup(func() { seam.failWithin("") })

		head := originCommitFile(t, origin.CodeBare, origin.primeBranch, "moved.txt", "one")
		logs.Reset()
		result, err := hubforge.OpenFabric(t, origin.Hub).Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != head {
			t.Errorf("Pull() WarpAdvanced = %v, NewWarpHEAD = %q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, head)
		}
		requireWarning(t, "SeamWarning", result.SeamWarning, origin.PrimeWorktree())
		requireWarning(t, "log", logs.String(), origin.PrimeWorktree())

		originCommitFile(t, origin.CodeBare, origin.primeBranch, "moved.txt", "two")
		var out bytes.Buffer
		if code := fabriccli.RunCLIIn(origin.PrimeWorktree(), &out, []string{"pull"}); code != 0 {
			t.Fatalf("fabric pull exit code = %d; output: %s", code, out.String())
		}
		if !strings.Contains(out.String(), `"seam_warning"`) || !strings.Contains(out.String(), seamHeal) {
			t.Errorf("fabric pull output = %s; want a seam_warning field naming %q", out.String(), seamHeal)
		}

		originCommitFile(t, origin.CodeBare, origin.primeBranch, "moved.txt", "three")
		hubResult, err := fabricengine.PullHub(prime, fabricengine.SyncOptions{}, func(*lymcwd.Location) (func(), error) { return func() {}, nil })
		if err != nil {
			t.Fatalf("PullHub() error = %v", err)
		}
		var primeSide *fabricengine.HubPullSide
		for i := range hubResult.Sides {
			if hubResult.Sides[i].Kind == fabricengine.HubPullPrime {
				primeSide = &hubResult.Sides[i]
			}
		}
		if primeSide == nil {
			t.Fatalf("PullHub() sides = %+v; want a prime side", hubResult.Sides)
		}
		requireWarning(t, "prime side detail", primeSide.Detail, origin.PrimeWorktree())
	})

	t.Run("A hub config that cannot be read keeps the event from being built and the pull still moves warp", func(t *testing.T) {
		fixturesDir := t.TempDir()
		f, _, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)
		if err := os.WriteFile(configengine.ConfigFile(weftFixture.BoardDir(), "fabric"), []byte("branch_prefix: [unclosed\n"), 0o644); err != nil {
			t.Fatalf("write the broken fabric config: %v", err)
		}
		newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

		result, err := f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != newTip {
			t.Errorf("Pull() WarpAdvanced = %v, NewWarpHEAD = %q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, newTip)
		}
		requireWarning(t, "SeamWarning", result.SeamWarning, weftFixture.PrimeWorktree())
		if events := seam.under(weftFixture.Path, seamTookIn); len(events) != 0 {
			t.Errorf("took-in events = %+v; want none, since the event could not be built", events)
		}
	})

	t.Run("The parent-pair fast-forward fires one took-in event, and a failing seam still returns the new head", func(t *testing.T) {
		origin := newOriginHub(t)
		fab := hubforge.OpenFabric(t, origin.Hub)
		head := originCommitFile(t, origin.CodeBare, origin.primeBranch, "AGENTS.md", "moved")

		result, err := fab.FastForwardPair(fabricengine.SyncOptions{SkipGit: true})
		if err != nil {
			t.Fatalf("FastForwardPair() error = %v", err)
		}
		event := requireOneEvent(t, seam, origin.Path, seamTookIn).event
		if result.NewHead != head || !slices.Equal(event.Paths, []string{"AGENTS.md"}) {
			t.Errorf("FastForwardPair() NewHead = %q, took-in paths = %v; want %q over [AGENTS.md]", result.NewHead, event.Paths, head)
		}

		seam.failWithin(origin.Path)
		t.Cleanup(func() { seam.failWithin("") })
		logs.Reset()
		head = originCommitFile(t, origin.CodeBare, origin.primeBranch, "AGENTS.md", "moved again")
		result, err = fab.FastForwardPair(fabricengine.SyncOptions{SkipGit: true})
		if err != nil || result.NewHead != head {
			t.Errorf("FastForwardPair() with a failing seam = (%q, %v); want NewHead %q and no error", result.NewHead, err, head)
		}
		requireWarning(t, "log", logs.String(), origin.PrimeWorktree())
	})

	t.Run("A reconcile that fast-forwards a pair's code branch fires one took-in event while its records branch and Bolt fire none, and a failing seam sets the outcome's warning", func(t *testing.T) {
		boardkit.InstallReplay(t)
		pair := newMovesPair(t)
		originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved/code.txt", "moved")
		originCommitFile(t, pair.RecordsBare, pair.recordsName, "moved/records.txt", "moved")
		originCommitFile(t, pair.RecordsBare, "main", "moved/bolt.txt", "moved")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})
		for _, row := range []struct{ repo, branch string }{{"code", pair.codeBranch}, {"records", pair.recordsName}, {"bolt", "main"}} {
			if outcome := requireState(t, report, row.repo, row.branch, fabricengine.BranchLevel); !outcome.Moved || outcome.Warning != "" {
				t.Errorf("%s outcome = %+v; want it moved with no warning", row.repo, outcome)
			}
		}
		event := requireOneEvent(t, seam, pair.Path, seamTookIn).event
		if event.Root != pair.code || !slices.Equal(event.Paths, []string{"moved/code.txt"}) {
			t.Errorf("took-in event = %+v; want root %q over [moved/code.txt]", event, pair.code)
		}

		seam.failWithin(pair.Path)
		t.Cleanup(func() { seam.failWithin("") })
		logs.Reset()
		originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved/again.txt", "moved")
		report = moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})
		outcome := requireState(t, report, "code", pair.codeBranch, fabricengine.BranchLevel)
		if !outcome.Moved {
			t.Errorf("code outcome = %+v; want it moved despite the seam failure", outcome)
		}
		requireWarning(t, "outcome Warning", outcome.Warning, pair.code)
		requireWarning(t, "log", logs.String(), pair.code)
		if !strings.Contains(report.Summary(), "warning: "+pair.codeBranch+": "+outcome.Warning) {
			t.Errorf("Summary() = %q; want the line %q", report.Summary(), "warning: "+pair.codeBranch+": "+outcome.Warning)
		}
	})
}

// lymcwdDotLym is the never-tracked directory name under a pair's anchor.
const lymcwdDotLym = ".lym"

// locationOf resolves the location of the worktree at root, failing the test on error.
func locationOf(t *testing.T, root string) *lymcwd.Location {
	t.Helper()
	l, err := lymcwd.ResolveWorktree(root)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s) error = %v", root, err)
	}
	return l
}
