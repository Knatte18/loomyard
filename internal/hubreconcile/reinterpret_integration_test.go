// reinterpret_integration_test.go covers the reinterpretations a stale-build walk applies to the board's hub base files: the drift dir rewrite on an anchored hub, its marker, and the one mail to the orch.

package hubreconcile_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
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

// recordNotices replaces the process's notifier with a recorder of the mails it is asked to send, and restores the previous one when the test ends.
func recordNotices(t *testing.T) *[]notice {
	t.Helper()

	return recordNoticesAnswering(t, true, nil)
}

// recordNoticesAnswering is recordNotices with a notifier that answers each mail with posted and err.
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

// driftMarkerRel is the board-relative path of the marker the drift dir reinterpretation writes.
func driftMarkerRel() string {
	return filepath.ToSlash(configengine.ReinterpretedMarkerRel(driftengine.DirBaseReinterpretation))
}

// driftYAMLWithDir returns a drift.yaml declaring capture and compare, each with dir when dir is not empty.
func driftYAMLWithDir(dir string) string {
	dirLine := ""
	if dir != "" {
		dirLine = "  dir: " + dir + "\n"
	}
	return "capture:\n  command: \"profile --out {baseline_dir}\"\n" + dirLine + "  timeout_min: 30\n" +
		"compare:\n  command: \"cmp {baseline_dir}\"\n" + dirLine + "  timeout_min: 5\n"
}

// driftDirs returns the capture and compare dirs the board's drift.yaml declares.
func driftDirs(t *testing.T, boardDir string) (capture, compare string) {
	t.Helper()

	cfg, err := driftengine.LoadConfig(configengine.Dirs{Board: boardDir})
	if err != nil {
		t.Fatalf("driftengine.LoadConfig over the board: %v", err)
	}
	return cfg.Capture.Dir, cfg.Compare.Dir
}

// It does not call t.Parallel: it installs the process-global notifier and swaps the package-global detached board sync.
func TestEnsure_DriftDirsOnAnAnchoredHubAreRewrittenOnceAndMailed(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"})
	geom := geometryOf(h)
	board := h.BoardDir()
	removeReinterpretationMarkers(t, board)
	gitkit.Git(t, board, "commit", "-q", "--allow-empty", "-m", "fixture: a board no reinterpretation was applied to")
	gitkit.CommitFile(t, board, configengine.ConfigFileRel("drift"), driftYAMLWithDir("."), "fixture: drift dirs written against the anchored directory")
	recordBoardSyncs(t)
	sent := recordNotices(t)

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if capture, compare := driftDirs(t, board); capture != h.Anchor || compare != h.Anchor {
		t.Errorf("drift dirs = %q and %q after the walk; want both rewritten to the anchor %q", capture, compare, h.Anchor)
	}
	committed := strings.Fields(gitkit.Git(t, board, "show", "--name-only", "--format=", "HEAD"))
	for _, want := range []string{filepath.ToSlash(configengine.ConfigFileRel("drift")), driftMarkerRel()} {
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
	for _, key := range []string{"capture.dir", "compare.dir"} {
		if !strings.Contains(got.line, key) {
			t.Errorf("notice line %q; want it to name %s", got.line, key)
		}
	}

	// The marker is present now: a drift.yaml written against the old base afterwards is left alone, and nothing is mailed.
	gitkit.CommitFile(t, board, configengine.ConfigFileRel("drift"), driftYAMLWithDir("."), "fixture: drift dirs set to . after the marker")
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if capture, compare := driftDirs(t, board); capture != "." || compare != "." {
		t.Errorf("drift dirs = %q and %q after the second walk; want them left alone beside the marker", capture, compare)
	}
	if len(*sent) != 1 {
		t.Errorf("notifier called %d times after the second walk; want no further mail", len(*sent))
	}
}

// It does not call t.Parallel: it installs the process-global notifier and swaps the package-global detached board sync.
func TestEnsure_ReinterpretationMarkerIsCommittedAndMailsOnlyAFlaggedValue(t *testing.T) {
	tiergate.Require(t, 1)

	tests := []struct {
		name   string
		anchor string
		// drift is the board's drift.yaml; empty leaves the hub's board as forged.
		drift string
		// wantDir is the capture and compare dir the file declares after the walk; ignored without a drift.yaml.
		wantDir string
		// wantMail is whether the walk mails the orch.
		wantMail bool
		// unposted makes the notifier report a mail not posted.
		unposted bool
		// notifierError makes the notifier fail.
		notifierError error
		// noNotifier leaves the process with no notifier installed, which holds once every earlier test has restored its own.
		noNotifier bool
	}{
		{name: "root-anchored hub with a dot dir", anchor: ".", drift: driftYAMLWithDir("."), wantDir: "."},
		{name: "anchored hub with no dir set", anchor: "backend", drift: driftYAMLWithDir(""), wantDir: ""},
		{name: "anchored hub with no drift.yaml", anchor: "backend"},
		{name: "anchored hub with a dir naming no directory is flagged and kept", anchor: "backend", drift: driftYAMLWithDir("nowhere"), wantDir: "nowhere", wantMail: true},
		{name: "a mail the notifier reports not posted leaves the walk done", anchor: "backend", drift: driftYAMLWithDir("nowhere"), wantDir: "nowhere", wantMail: true, unposted: true},
		{name: "a notifier error leaves the walk done", anchor: "backend", drift: driftYAMLWithDir("nowhere"), wantDir: "nowhere", wantMail: true, notifierError: errors.New("creel is down")},
		{name: "no installed notifier leaves the walk done", anchor: "backend", drift: driftYAMLWithDir("nowhere"), wantDir: "nowhere", noNotifier: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: tt.anchor})
			board := h.BoardDir()
			removeReinterpretationMarkers(t, board)
			gitkit.Git(t, board, "commit", "-q", "--allow-empty", "-m", "fixture: a board no reinterpretation was applied to")
			if tt.drift != "" {
				gitkit.CommitFile(t, board, configengine.ConfigFileRel("drift"), tt.drift, "fixture: drift.yaml")
			}
			recordBoardSyncs(t)
			sent := new([]notice)
			if !tt.noNotifier {
				sent = recordNoticesAnswering(t, !tt.unposted, tt.notifierError)
			}

			if err := hubreconcile.Ensure(geometryOf(h), hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure: %v", err)
			}

			if !slices.Contains(gitkit.LsFiles(t, board), driftMarkerRel()) {
				t.Errorf("the board does not track %s after the walk", driftMarkerRel())
			}
			assertClean(t, board)
			if tt.drift != "" {
				if capture, compare := driftDirs(t, board); capture != tt.wantDir || compare != tt.wantDir {
					t.Errorf("drift dirs = %q and %q; want %q and %q", capture, compare, tt.wantDir, tt.wantDir)
				}
			}
			if gotMail := len(*sent) > 0; gotMail != tt.wantMail {
				t.Fatalf("notifier called %d times; want a mail: %v", len(*sent), tt.wantMail)
			}
			if tt.wantMail {
				if len(*sent) != 1 || !strings.Contains((*sent)[0].line, "capture.dir") || !strings.Contains((*sent)[0].line, "compare.dir") {
					t.Errorf("notices = %v; want one line naming capture.dir and compare.dir", *sent)
				}
			}
		})
	}
}
