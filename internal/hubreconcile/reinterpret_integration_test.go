// reinterpret_integration_test.go covers the reinterpretations a stale-build walk applies to the board's hub base files.
// No module registers a reinterpretation of its own, so the test registers a test-only one on the slack module:
// the rewrite, its marker, and the one mail to the orch.

package hubreconcile_test

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeReinterpretationID names the test-only reinterpretation of the slack module.
const fakeReinterpretationID = "test-slack-reminder-seconds"

// fakeMarkerRel is the board-relative path of the marker the test-only reinterpretation writes.
func fakeMarkerRel() string {
	return filepath.ToSlash(configengine.ReinterpretedMarkerRel(fakeReinterpretationID))
}

// fakeRewrite is the test-only reinterpretation of slack.yaml.
// It rewrites a remind_after_min of 10 to 600 and flags a remind_after_min of 0, which it leaves as it is.
func fakeRewrite(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	switch {
	case bytes.Contains(existing, []byte("remind_after_min: 10\n")):
		rewritten = bytes.ReplaceAll(existing, []byte("remind_after_min: 10\n"), []byte("remind_after_min: 600\n"))
		return rewritten, []string{"remind_after_min 10 rewritten to 600"}, nil, nil
	case bytes.Contains(existing, []byte("remind_after_min: 0\n")):
		return existing, nil, []string{"remind_after_min 0 needs a decision; way forward: set it with lym config slack"}, nil
	}
	return existing, nil, nil, nil
}

// registerFakeReinterpretation makes the walk apply the test-only reinterpretation to slack.yaml.
// It restores the real registry when the test ends.
func registerFakeReinterpretation(t *testing.T) {
	t.Helper()

	t.Cleanup(hubreconcile.SetReinterpretationModulesForTest(func() []configreg.Module {
		modules := configreg.Modules()
		for i := range modules {
			if modules[i].Name == "slack" {
				modules[i].Reinterpretations = []configreg.Reinterpretation{{ID: fakeReinterpretationID, Rewrite: fakeRewrite}}
			}
		}
		return modules
	}))
}

// TestEnsure_Reinterpretation does not call t.Parallel:
// it installs the process-global registry, whose test-only reinterpretation every other walk of the binary would apply.
// Its subtests share that registry and run in parallel, bar the one that removes the process's notifier.
func TestEnsure_Reinterpretation(t *testing.T) {
	tiergate.Require(t, 1)
	registerFakeReinterpretation(t)

	t.Run("no installed notifier leaves the walk done", func(t *testing.T) {
		uninstallOrchMail()
		t.Cleanup(func() { uninstallOrchMail = hubreconcile.InstallNotifier(orchMail.send) })
		runMarkedWalk(t, markedWalk{slack: "remind_after_min: 0\n"})
	})
	t.Run("a rewritten value is rewritten once and mailed", func(t *testing.T) {
		t.Parallel()
		runRewrittenOnce(t)
	})
	t.Run("a failed walk leaves no marker and the next walk commits it once", func(t *testing.T) {
		t.Parallel()
		runFailedWalkAfterAReinterpretation(t)
	})
	for _, tt := range []struct {
		name string
		walk markedWalk
	}{
		{name: "a value the rewrite leaves alone is marked and not mailed", walk: markedWalk{slack: "remind_after_min: 5\n"}},
		{name: "a flagged value is kept and mailed", walk: markedWalk{slack: "remind_after_min: 0\n", wantLine: "remind_after_min 0 needs a decision"}},
		{
			name: "a mail the notifier reports not posted leaves the walk done",
			walk: markedWalk{slack: "remind_after_min: 0\n", wantLine: "remind_after_min 0 needs a decision", answer: mailAnswer{unposted: true}},
		},
		{
			name: "a notifier error leaves the walk done",
			walk: markedWalk{slack: "remind_after_min: 0\n", wantLine: "remind_after_min 0 needs a decision", answer: mailAnswer{err: errors.New("creel is down")}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runMarkedWalk(t, tt.walk)
		})
	}
}

// runRewrittenOnce walks a board whose slack.yaml holds the value the reinterpretation rewrites.
// It asserts the rewrite, the marker and the one mail.
// It then asserts that a later walk leaves the old value alone beside the marker and mails nothing.
func runRewrittenOnce(t *testing.T) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	board := h.BoardDir()
	slackRel := configengine.ConfigFileRel("slack")
	gitkit.CommitFile(t, board, slackRel, "remind_after_min: 10\n", "fixture: slack.yaml holding the value the reinterpretation rewrites")

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if got, err := os.ReadFile(configengine.ConfigFile(board, "slack")); err != nil || string(got) != "remind_after_min: 600\n" {
		t.Errorf("board slack.yaml = %q, %v after the walk; want the value rewritten to 600", got, err)
	}
	committed := strings.Fields(gitkit.Git(t, board, "show", "--name-only", "--format=", "HEAD"))
	for _, want := range []string{filepath.ToSlash(slackRel), fakeMarkerRel()} {
		if !slices.Contains(committed, want) {
			t.Errorf("the walk's board commit holds %v; want it to carry %s", committed, want)
		}
	}
	assertClean(t, board)
	mailed := orchMail.lines(board)
	if len(mailed) != 1 {
		t.Fatalf("notifier called %d times; want once: %v", len(mailed), mailed)
	}
	if !strings.HasPrefix(mailed[0], "[reconcile notice]") {
		t.Errorf("notice line %q; want it to open with [reconcile notice]", mailed[0])
	}
	if !strings.Contains(mailed[0], "slack.yaml remind_after_min 10 rewritten to 600") {
		t.Errorf("notice line %q; want it to name the rewritten value of slack.yaml", mailed[0])
	}

	// With the marker present, a slack.yaml written with the old value afterwards is left alone, and nothing is mailed.
	gitkit.CommitFile(t, board, slackRel, "remind_after_min: 10\n", "fixture: the old value again after the marker")
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if got, err := os.ReadFile(configengine.ConfigFile(board, "slack")); err != nil || string(got) != "remind_after_min: 10\n" {
		t.Errorf("board slack.yaml = %q, %v after the second walk; want it left alone beside the marker", got, err)
	}
	if got := orchMail.lines(board); len(got) != 1 {
		t.Errorf("notifier called %d times after the second walk; want no further mail", len(got))
	}
}

// runFailedWalkAfterAReinterpretation fails a walk once the hub-wide files, a template base and the marker are written.
// It asserts the failure names the board and restores its config files.
// That restore removes the marker and the base, and leaves the board's status and the stamp as they were.
// The next walk then commits the marker once, stamps the build and leaves the board clean.
func runFailedWalkAfterAReinterpretation(t *testing.T) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	board := h.BoardDir()
	gitkit.CommitFile(t, board, configengine.ConfigFileRel("slack"), "remind_after_min: 10\n", "fixture: slack.yaml holding the value the reinterpretation rewrites")
	// The profiles template base is dropped from the board, so the failed walk writes it and must remove it again.
	gitkit.Git(t, board, "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.TemplateBaseFileRel("profiles")))
	gitkit.Git(t, board, "commit", "-q", "--allow-empty", "-m", "fixture: no profiles template base")
	filesBefore := configFilesOf(t, board)
	statusBefore := gitkit.GitStatusPorcelain(t, board)

	err := hubreconcile.EnsureFailingAfterHubWideWrite(geom, hubreconcile.Options{}, errors.New("fixture: failure after the hub-wide write"))

	var worktreeErr *hubreconcile.WorktreeError
	if !errors.As(err, &worktreeErr) {
		t.Fatalf("EnsureFailingAfterHubWideWrite error = %v; want *WorktreeError", err)
	}
	if worktreeErr.Worktree != board {
		t.Errorf("WorktreeError names worktree %q; want the board %q", worktreeErr.Worktree, board)
	}
	if filesAfter := configFilesOf(t, board); !maps.Equal(filesBefore, filesAfter) {
		t.Errorf("the failed walk changed the board's config files:\nbefore: %v\nafter: %v", filesBefore, filesAfter)
	}
	for _, path := range []string{configengine.ReinterpretedMarker(board, fakeReinterpretationID), configengine.TemplateBaseFile(board, "profiles")} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s exists after the failed walk (stat error %v); want it removed", path, statErr)
		}
	}
	if status := gitkit.GitStatusPorcelain(t, board); status != statusBefore {
		t.Errorf("board status changed by the failed walk:\nbefore:\n%s\nafter:\n%s", statusBefore, status)
	}
	if _, found := stampKey(t, geom); found {
		t.Errorf("stamp written after a failed walk")
	}

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure after the failed walk: %v", err)
	}
	if commits := strings.Fields(gitkit.Git(t, board, "log", "--format=%H", "--", fakeMarkerRel())); len(commits) != 1 {
		t.Errorf("commits touching %s = %v; want exactly one", fakeMarkerRel(), commits)
	}
	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent after the walk that followed the failed one")
	}
	assertClean(t, board)
}

// markedWalk is the board a walk that applies the reinterpretation starts from, and what it expects.
type markedWalk struct {
	// slack is the board's slack.yaml, which the walk is expected to leave as it is.
	slack string
	// wantLine is the text the one mailed line must hold; empty expects no mail.
	wantLine string
	// answer is how the notifier answers the board's mail.
	answer mailAnswer
}

// runMarkedWalk walks a board holding walk's slack.yaml and asserts that the walk ends done.
// The board tracks the marker, is clean, keeps slack.yaml, and its orch got the mail walk expects.
func runMarkedWalk(t *testing.T, walk markedWalk) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	board := h.BoardDir()
	gitkit.CommitFile(t, board, configengine.ConfigFileRel("slack"), walk.slack, "fixture: slack.yaml")
	orchMail.answer(board, walk.answer)

	if err := hubreconcile.Ensure(geometryOf(h), hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if !slices.Contains(gitkit.LsFiles(t, board), fakeMarkerRel()) {
		t.Errorf("the board does not track %s after the walk", fakeMarkerRel())
	}
	assertClean(t, board)
	if got, err := os.ReadFile(configengine.ConfigFile(board, "slack")); err != nil || string(got) != walk.slack {
		t.Errorf("board slack.yaml = %q, %v; want %q", got, err, walk.slack)
	}
	mailed := orchMail.lines(board)
	if walk.wantLine == "" && len(mailed) != 0 {
		t.Errorf("notices = %q; want none", mailed)
	}
	if walk.wantLine != "" && (len(mailed) != 1 || !strings.Contains(mailed[0], walk.wantLine)) {
		t.Errorf("notices = %q; want one line holding %q", mailed, walk.wantLine)
	}
}
