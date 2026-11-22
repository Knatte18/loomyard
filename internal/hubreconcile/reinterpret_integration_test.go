// reinterpret_integration_test.go covers the reinterpretations a stale-build walk applies to the board's hub base files.
// No module registers a reinterpretation of its own, so each test registers a test-only one on the slack module:
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
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// notice is one mail a walk sent through the installed notifier.
type notice struct {
	boardDir string
	line     string
}

// recordNotices replaces the process's notifier with a recorder of the mails it is asked to send.
// It restores the previous notifier when the test ends.
func recordNotices(t *testing.T) *[]notice {
	t.Helper()

	return recordNoticesAnswering(t, true, nil)
}

// recordNoticesAnswering replaces the process's notifier with a recorder that answers each mail with posted and err.
func recordNoticesAnswering(t *testing.T, posted bool, err error) *[]notice {
	t.Helper()

	var mu sync.Mutex
	var sent []notice
	t.Cleanup(hubreconcile.InstallNotifier(func(boardDir, line string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, notice{boardDir: boardDir, line: line})
		return posted, err
	}))
	return &sent
}

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

// TestEnsure_ReinterpretedValueIsRewrittenOnceAndMailed does not call t.Parallel:
// it installs the process-global notifier and registry and swaps the package-global detached board sync.
func TestEnsure_ReinterpretedValueIsRewrittenOnceAndMailed(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	board := h.BoardDir()
	slackRel := configengine.ConfigFileRel("slack")
	gitkit.CommitFile(t, board, slackRel, "remind_after_min: 10\n", "fixture: slack.yaml holding the value the reinterpretation rewrites")
	registerFakeReinterpretation(t)
	recordBoardSyncs(t)
	sent := recordNotices(t)

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
	if len(*sent) != 1 {
		t.Fatalf("notifier called %d times; want once: %v", len(*sent), *sent)
	}
	got := (*sent)[0]
	if got.boardDir != board {
		t.Errorf("notice sent for board %q; want %q", got.boardDir, board)
	}
	if !strings.HasPrefix(got.line, "[reconcile notice]") {
		t.Errorf("notice line %q; want it to open with [reconcile notice]", got.line)
	}
	if !strings.Contains(got.line, "slack.yaml remind_after_min 10 rewritten to 600") {
		t.Errorf("notice line %q; want it to name the rewritten value of slack.yaml", got.line)
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
	if len(*sent) != 1 {
		t.Errorf("notifier called %d times after the second walk; want no further mail", len(*sent))
	}
}

// TestEnsure_FailedWalkAfterAReinterpretationLeavesNoMarkerAndTheNextWalkCommitsItOnce does not call t.Parallel:
// it installs the process-global notifier and registry and swaps the package-global detached board sync.
func TestEnsure_FailedWalkAfterAReinterpretationLeavesNoMarkerAndTheNextWalkCommitsItOnce(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	board := h.BoardDir()
	gitkit.CommitFile(t, board, configengine.ConfigFileRel("slack"), "remind_after_min: 10\n", "fixture: slack.yaml holding the value the reinterpretation rewrites")
	// The profiles template base is dropped from the board, so the failed walk writes it and must remove it again.
	gitkit.Git(t, board, "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.TemplateBaseFileRel("profiles")))
	gitkit.Git(t, board, "commit", "-q", "--allow-empty", "-m", "fixture: no profiles template base")
	registerFakeReinterpretation(t)
	recordBoardSyncs(t)
	recordNotices(t)
	filesBefore := configFilesOf(t, board)

	err := hubreconcile.EnsureFailingAfterHubWideWrite(geom, hubreconcile.Options{}, errors.New("fixture: failure after the hub-wide write"))

	var worktreeErr *hubreconcile.WorktreeError
	if !errors.As(err, &worktreeErr) {
		t.Fatalf("EnsureFailingAfterHubWideWrite error = %v; want *WorktreeError", err)
	}
	if filesAfter := configFilesOf(t, board); !maps.Equal(filesBefore, filesAfter) {
		t.Errorf("the failed walk changed the board's config files:\nbefore: %v\nafter: %v", filesBefore, filesAfter)
	}
	for _, path := range []string{configengine.ReinterpretedMarker(board, fakeReinterpretationID), configengine.TemplateBaseFile(board, "profiles")} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s exists after the failed walk (stat error %v); want it removed", path, statErr)
		}
	}

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure after the failed walk: %v", err)
	}
	if commits := strings.Fields(gitkit.Git(t, board, "log", "--format=%H", "--", fakeMarkerRel())); len(commits) != 1 {
		t.Errorf("commits touching %s = %v; want exactly one", fakeMarkerRel(), commits)
	}
}

// TestEnsure_ReinterpretationMarkerIsCommittedAndMailsOnlyWhatItRewroteOrFlagged does not call t.Parallel:
// it installs the process-global notifier and registry and swaps the package-global detached board sync.
func TestEnsure_ReinterpretationMarkerIsCommittedAndMailsOnlyWhatItRewroteOrFlagged(t *testing.T) {
	tiergate.Require(t, 1)

	tests := []struct {
		name string
		// slack is the board's slack.yaml; empty leaves the hub's board as forged.
		slack string
		// wantSlack is the slack.yaml the board holds after the walk; ignored when slack is empty.
		wantSlack string
		// wantMail is whether the walk mails the orch.
		wantMail bool
		// wantLine is the text the one mailed line must hold.
		wantLine string
		// unposted makes the notifier report a mail not posted.
		unposted bool
		// notifierError makes the notifier fail.
		notifierError error
		// noNotifier leaves the process with no notifier installed, which holds once every earlier test has restored its own.
		noNotifier bool
	}{
		{name: "a hub file with no value to carry is only marked"},
		{name: "a value the rewrite leaves alone is marked and not mailed", slack: "remind_after_min: 5\n", wantSlack: "remind_after_min: 5\n"},
		{name: "a rewritten value is mailed", slack: "remind_after_min: 10\n", wantSlack: "remind_after_min: 600\n", wantMail: true, wantLine: "remind_after_min 10 rewritten to 600"},
		{name: "a flagged value is kept and mailed", slack: "remind_after_min: 0\n", wantSlack: "remind_after_min: 0\n", wantMail: true, wantLine: "remind_after_min 0 needs a decision"},
		{name: "a mail the notifier reports not posted leaves the walk done", slack: "remind_after_min: 0\n", wantSlack: "remind_after_min: 0\n", wantMail: true, wantLine: "remind_after_min 0 needs a decision", unposted: true},
		{name: "a notifier error leaves the walk done", slack: "remind_after_min: 0\n", wantSlack: "remind_after_min: 0\n", wantMail: true, wantLine: "remind_after_min 0 needs a decision", notifierError: errors.New("creel is down")},
		{name: "no installed notifier leaves the walk done", slack: "remind_after_min: 0\n", wantSlack: "remind_after_min: 0\n", noNotifier: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			board := h.BoardDir()
			if tt.slack != "" {
				gitkit.CommitFile(t, board, configengine.ConfigFileRel("slack"), tt.slack, "fixture: slack.yaml")
			}
			registerFakeReinterpretation(t)
			recordBoardSyncs(t)
			sent := new([]notice)
			if !tt.noNotifier {
				sent = recordNoticesAnswering(t, !tt.unposted, tt.notifierError)
			}

			if err := hubreconcile.Ensure(geometryOf(h), hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure: %v", err)
			}

			if !slices.Contains(gitkit.LsFiles(t, board), fakeMarkerRel()) {
				t.Errorf("the board does not track %s after the walk", fakeMarkerRel())
			}
			assertClean(t, board)
			if tt.slack != "" {
				if got, err := os.ReadFile(configengine.ConfigFile(board, "slack")); err != nil || string(got) != tt.wantSlack {
					t.Errorf("board slack.yaml = %q, %v; want %q", got, err, tt.wantSlack)
				}
			}
			if gotMail := len(*sent) > 0; gotMail != tt.wantMail {
				t.Fatalf("notifier called %d times; want a mail: %v", len(*sent), tt.wantMail)
			}
			if tt.wantMail {
				if len(*sent) != 1 || !strings.Contains((*sent)[0].line, tt.wantLine) {
					t.Errorf("notices = %v; want one line holding %q", *sent, tt.wantLine)
				}
			}
		})
	}
}
