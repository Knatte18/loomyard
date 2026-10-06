//go:build integration

// reset_test.go covers the reset verb through its cobra.Command over one hubforge hub, each step on its own task pair.
// Each success path checks the branch, the files and the mutation record; each refusal checks its way forward and that nothing moved.
// WEFT_SKIP_GIT=1 is set for the whole scenario, so the closing fabric sync commits nothing and needs no records sibling beyond the pair's own.

package webstercli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// resetFixture is a task pair's code worktree with a base commit, a later commit and a webster CLI wired over it.
type resetFixture struct {
	cli      *websterCLI
	checkout string
	branch   string
	base     string
}

// newResetFixture adds a pair for slug to h and commits own.txt (the base) and later.txt on its branch.
// The CLI carries a fake engine whose parent transcript records a successful write of own.txt.
func newResetFixture(t *testing.T, h *hubforge.Hub, slug string) *resetFixture {
	t.Helper()

	hubforge.AddPair(t, h, slug)
	checkout := h.PairWarpWorktree(slug)
	loc, err := lyxcwd.ResolveWorktree(checkout)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", checkout, err)
	}

	base := gitkit.CommitFile(t, checkout, "own.txt", "base", "base commit")
	gitkit.CommitFile(t, checkout, "later.txt", "later", "later commit")

	engine := &shuttlefake.Engine{AuditForksFn: func(string, string) (shuttleengine.ForkAudit, error) {
		return shuttleengine.ForkAudit{ParentWriteEvents: []shuttleengine.WriteEvent{
			{Path: filepath.Join(checkout, "own.txt"), Succeeded: true},
		}}, nil
	}}
	c := &websterCLI{
		engine:     engine,
		anchorRel:  loc.AnchorRel,
		geom:       hubgeom.WebsterGeometry(loc),
		openFabric: func() (*fabricengine.Fabric, error) { return fabricengine.Open(loc) },
		parentBranch: func() (string, error) {
			origin, found, err := fabricengine.ReadOrigin(loc)
			if err != nil || !found {
				return "", err
			}
			return origin.ParentBranch, nil
		},
	}
	return &resetFixture{
		cli:      c,
		checkout: checkout,
		branch:   gitkit.Git(t, checkout, "rev-parse", "--abbrev-ref", "HEAD"),
		base:     base,
	}
}

// saveState records st in the fixture's webster dir.
func (fx *resetFixture) saveState(t *testing.T, st *websterengine.State) {
	t.Helper()
	if err := websterengine.SaveState(fx.cli.geom.WebsterDir, fx.cli.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
}

// startedAt is a state whose one batch began at sha.
func startedAt(sha string) *websterengine.State {
	return &websterengine.State{
		MasterSessionID: "master-session",
		Batches:         map[int]*websterengine.BatchState{1: {Slug: "only", StartSHA: sha}},
	}
}

// reset runs `reset` with args and returns the exit code and the decoded envelope.
// A nil args becomes an empty slice, which keeps cobra from reading the test binary's own flags.
func (fx *resetFixture) reset(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	if args == nil {
		args = []string{}
	}
	var out strings.Builder
	code := clihelp.Execute(fx.cli.resetCmd(), &out, args)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &envelope); err != nil {
		t.Fatalf("reset %v output %q is not one JSON envelope: %v", args, out.String(), err)
	}
	return code, envelope
}

// wantRefusal runs `reset` with args, and fails unless it refuses with every part in its error and leaves HEAD at the later commit.
func (fx *resetFixture) wantRefusal(t *testing.T, args []string, parts ...string) {
	t.Helper()
	head := gitkit.RevParse(t, fx.checkout, "HEAD")
	code, envelope := fx.reset(t, args...)
	msg, _ := envelope["error"].(string)
	if code == 0 || envelope["ok"] != false {
		t.Fatalf("reset %v = %d, %v; want a refusal containing %q", args, code, envelope, parts)
	}
	for _, p := range parts {
		if !strings.Contains(msg, p) {
			t.Errorf("reset %v error = %q; want it to contain %q", args, msg, p)
		}
	}
	if _, has := envelope["mutations"]; has {
		t.Errorf("refusal envelope carries mutations: %v", envelope)
	}
	if got := gitkit.RevParse(t, fx.checkout, "HEAD"); got != head {
		t.Errorf("a refused reset moved HEAD from %s to %s", head, got)
	}
}

// TestResetCmd runs every reset case as a step over one hub, each step adding its own pair so no step relies on another's state.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestResetCmd(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	h := hubforge.NewHub(t, ".")

	if !t.Run("start resets head and own dirt and keeps untracked", func(t *testing.T) {
		fx := newResetFixture(t, h, "rst-start")
		fx.saveState(t, startedAt(fx.base))
		if err := os.WriteFile(filepath.Join(fx.checkout, "own.txt"), []byte("dirty"), 0o644); err != nil {
			t.Fatal(err)
		}
		untracked := filepath.Join(fx.checkout, "untracked.txt")
		if err := os.WriteFile(untracked, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}

		code, envelope := fx.reset(t, "--to", "start")
		if code != 0 || envelope["ok"] != true {
			t.Fatalf("reset --to start = %d, %v; want ok", code, envelope)
		}
		if envelope["target"] != "start" || envelope["sha"] != fx.base || envelope["partial"] != false {
			t.Errorf("envelope = %v; want target start, sha %s, partial false", envelope, fx.base)
		}
		mutations, _ := envelope["mutations"].([]any)
		if len(mutations) != 1 {
			t.Fatalf("mutations = %v; want exactly one worktree_reset entry", envelope["mutations"])
		}
		if entry, _ := mutations[0].(map[string]any); entry["kind"] != string(fabricengine.KindWorktreeReset) {
			t.Errorf("mutation = %v; want kind %s", mutations[0], fabricengine.KindWorktreeReset)
		}
		if got := gitkit.RevParse(t, fx.checkout, "HEAD"); got != fx.base {
			t.Errorf("HEAD = %s; want %s", got, fx.base)
		}
		if _, err := os.Stat(filepath.Join(fx.checkout, "later.txt")); !os.IsNotExist(err) {
			t.Errorf("later.txt survived the reset (err = %v)", err)
		}
		if got, err := os.ReadFile(filepath.Join(fx.checkout, "own.txt")); err != nil || string(got) != "base" {
			t.Errorf("own.txt = %q, %v; want %q", got, err, "base")
		}
		if _, err := os.Stat(untracked); err != nil {
			t.Errorf("untracked file removed by the reset: %v", err)
		}
	}) {
		return
	}

	if !t.Run("pre-fix resets and clears the pre-fix head", func(t *testing.T) {
		fx := newResetFixture(t, h, "rst-prefix")
		st := startedAt(fx.base)
		st.PreFixHead = fx.base
		fx.saveState(t, st)
		gitkit.CommitFile(t, fx.checkout, "rejected-fix.txt", "fix", "rejected fixer commit")

		code, envelope := fx.reset(t, "--to", "pre-fix")
		if code != 0 || envelope["ok"] != true || envelope["target"] != "pre-fix" {
			t.Fatalf("reset --to pre-fix = %d, %v; want ok at pre-fix", code, envelope)
		}
		if got := gitkit.RevParse(t, fx.checkout, "HEAD"); got != fx.base {
			t.Errorf("HEAD = %s; want the pre-fix head %s", got, fx.base)
		}
		loaded, err := websterengine.LoadState(fx.cli.geom.WebsterDir, fx.cli.geom.ScratchDir)
		if err != nil || loaded == nil {
			t.Fatalf("LoadState = %v, %v", loaded, err)
		}
		if loaded.PreFixHead != "" {
			t.Errorf("PreFixHead = %q after the reset; want it cleared", loaded.PreFixHead)
		}
	}) {
		return
	}

	if !t.Run("start resolves the octopus merge base of diverging starts", func(t *testing.T) {
		fx := newResetFixture(t, h, "rst-octopus")
		gitkit.Git(t, fx.checkout, "checkout", "-b", "rst-octopus-s1", fx.base)
		s1 := gitkit.CommitFile(t, fx.checkout, "s1.txt", "1", "s1")
		gitkit.Git(t, fx.checkout, "checkout", "-b", "rst-octopus-s2", fx.base)
		s2 := gitkit.CommitFile(t, fx.checkout, "s2.txt", "2", "s2")
		gitkit.Git(t, fx.checkout, "checkout", fx.branch)
		fx.saveState(t, &websterengine.State{
			MasterSessionID: "master-session",
			Batches: map[int]*websterengine.BatchState{
				1: {Slug: "one", StartSHA: s1},
				2: {Slug: "two", StartSHA: s2},
			},
		})

		code, envelope := fx.reset(t, "--to", "start")
		if code != 0 || envelope["sha"] != fx.base {
			t.Fatalf("reset --to start = %d, %v; want ok at the merge-base %s", code, envelope, fx.base)
		}
		if got := gitkit.RevParse(t, fx.checkout, "HEAD"); got != fx.base {
			t.Errorf("HEAD = %s; want %s", got, fx.base)
		}
	}) {
		return
	}

	refusals := []struct {
		name string
		// arrange sets the fixture up for the refusal; the fixture's state starts empty.
		arrange func(t *testing.T, fx *resetFixture)
		// attempts are the verb invocations, each refused with every part of its wantIn.
		attempts []refusalAttempt
	}{
		{
			name: "unknown target names both targets",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
			},
			attempts: []refusalAttempt{
				{[]string{"--to", "bogus"}, []string{`"bogus"`, "lyx webster reset --to start", "lyx webster reset --to pre-fix"}},
				{nil, []string{"lyx webster reset --to start", "lyx webster reset --to pre-fix"}},
			},
		},
		{
			name: "run busy names status",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				if err := os.MkdirAll(fx.cli.geom.ScratchDir, 0o755); err != nil {
					t.Fatal(err)
				}
				held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(fx.cli.geom.ScratchDir, "run.lock"))
				if err != nil || !acquired {
					t.Fatalf("hold run.lock = %v, %v", acquired, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"run.lock held", "lyx webster status"}}},
		},
		{
			name:     "no run",
			arrange:  func(*testing.T, *resetFixture) {},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"no state.json", "run `lyx webster run` first"}}},
		},
		{
			name: "merge in progress",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				mergeHead := gitkit.Git(t, fx.checkout, "rev-parse", "--path-format=absolute", "--git-path", "MERGE_HEAD")
				if err := os.WriteFile(strings.TrimSpace(mergeHead), []byte(fx.base+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"merge in progress", "lyx fabric merge --abort"}}},
		},
		{
			name: "detached head",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				gitkit.Git(t, fx.checkout, "checkout", "--detach")
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"git switch <task-branch>", "re-run `lyx webster reset --to start`"}}},
		},
		{
			name: "parent branch",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				fx.cli.parentBranch = func() (string, error) { return fx.branch, nil }
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"parent branch", "git switch <task-branch>"}}},
		},
		{
			name: "non-pair branch refused by fabric",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				gitkit.Git(t, fx.checkout, "checkout", "-b", "not-the-pair-branch")
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"reset --to start refused"}}},
		},
		{
			name: "no recorded target",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, &websterengine.State{MasterSessionID: "master-session", Batches: map[int]*websterengine.BatchState{}})
			},
			attempts: []refusalAttempt{
				{[]string{"--to", "start"}, []string{"no batch recorded a start commit", "lyx webster run --fresh"}},
				{[]string{"--to", "pre-fix"}, []string{"no pre-fix head", "run `lyx webster run`"}},
			},
		},
		{
			name: "missing target commit",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(strings.Repeat("ab", 20)))
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{strings.Repeat("ab", 20), "not in this repository", "fetch the task branch"}}},
		},
		{
			name: "target not an ancestor",
			arrange: func(t *testing.T, fx *resetFixture) {
				gitkit.Git(t, fx.checkout, "checkout", "-b", "rst-notancestor-side", fx.base)
				side := gitkit.CommitFile(t, fx.checkout, "side.txt", "side", "side")
				gitkit.Git(t, fx.checkout, "checkout", fx.branch)
				st := startedAt(side)
				st.PreFixHead = side
				fx.saveState(t, st)
			},
			attempts: []refusalAttempt{
				{[]string{"--to", "start"}, []string{"not an ancestor of HEAD", "lyx webster run --fresh"}},
				{[]string{"--to", "pre-fix"}, []string{"not an ancestor of HEAD", "lyx webster reset --to start"}},
			},
		},
		{
			name: "dirty path the run did not write",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				if err := os.WriteFile(filepath.Join(fx.checkout, "later.txt"), []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"later.txt", "git checkout -- <path>", "re-run `lyx webster reset --to start`"}}},
		},
		{
			name: "standalone names git reset keep",
			arrange: func(t *testing.T, fx *resetFixture) {
				fx.saveState(t, startedAt(fx.base))
				fx.cli.openFabric = nil
				fx.cli.parentBranch = nil
			},
			attempts: []refusalAttempt{{[]string{"--to", "start"}, []string{"standalone", "git reset --keep {base}"}}},
		},
	}
	for i, tc := range refusals {
		if !t.Run("refuses "+tc.name, func(t *testing.T) {
			fx := newResetFixture(t, h, "rst-refuse-"+string(rune('a'+i)))
			tc.arrange(t, fx)
			for _, attempt := range tc.attempts {
				parts := make([]string, len(attempt.wantIn))
				for j, part := range attempt.wantIn {
					parts[j] = strings.ReplaceAll(part, "{base}", fx.base)
				}
				fx.wantRefusal(t, attempt.args, parts...)
			}
		}) {
			return
		}
	}
}

// refusalAttempt is one reset invocation and the parts its refusal must carry.
type refusalAttempt struct {
	args   []string
	wantIn []string
}
