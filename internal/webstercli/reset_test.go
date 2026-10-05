//go:build integration

// reset_test.go covers the reset verb through its cobra.Command over a hubforge hub with a task pair.
// Each success path checks the branch, the files and the mutation record; each refusal checks its way forward and that nothing moved.
// WEFT_SKIP_GIT=1 is set on every test, so the closing fabric sync commits nothing and needs no records sibling beyond the pair's own.

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

// newResetFixture adds a pair for slug and commits own.txt (the base) and later.txt on its branch.
// The CLI carries a fake engine whose parent transcript records a successful write of own.txt.
func newResetFixture(t *testing.T, slug string) *resetFixture {
	t.Helper()
	t.Setenv("WEFT_SKIP_GIT", "1")

	h := hubforge.NewHub(t, ".")
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
func (fx *resetFixture) reset(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
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

func TestResetCmd_StartResetsHeadAndOwnDirtKeepsUntracked(t *testing.T) {
	fx := newResetFixture(t, "rst-start")
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
}

func TestResetCmd_PreFixResetsAndClearsPreFixHead(t *testing.T) {
	fx := newResetFixture(t, "rst-prefix")
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
}

func TestResetCmd_StartResolvesOctopusMergeBaseOfDivergingStarts(t *testing.T) {
	fx := newResetFixture(t, "rst-octopus")
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
}

func TestResetCmd_UnknownToRefusesNamingBothTargets(t *testing.T) {
	fx := newResetFixture(t, "rst-unknown")
	fx.saveState(t, startedAt(fx.base))
	fx.wantRefusal(t, []string{"--to", "bogus"}, `"bogus"`, "lyx webster reset --to start", "lyx webster reset --to pre-fix")
	fx.wantRefusal(t, nil, "lyx webster reset --to start", "lyx webster reset --to pre-fix")
}

func TestResetCmd_RunBusyRefusesNamingStatus(t *testing.T) {
	fx := newResetFixture(t, "rst-busy")
	fx.saveState(t, startedAt(fx.base))
	if err := os.MkdirAll(fx.cli.geom.ScratchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(fx.cli.geom.ScratchDir, "run.lock"))
	if err != nil || !acquired {
		t.Fatalf("hold run.lock = %v, %v", acquired, err)
	}
	defer func() { _ = held.Release() }()
	fx.wantRefusal(t, []string{"--to", "start"}, "run.lock held", "lyx webster status")
}

func TestResetCmd_NoRunRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-norun")
	fx.wantRefusal(t, []string{"--to", "start"}, "no state.json", "run `lyx webster run` first")
}

func TestResetCmd_MergeInProgressRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-merge")
	fx.saveState(t, startedAt(fx.base))
	mergeHead := gitkit.Git(t, fx.checkout, "rev-parse", "--path-format=absolute", "--git-path", "MERGE_HEAD")
	if err := os.WriteFile(strings.TrimSpace(mergeHead), []byte(fx.base+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fx.wantRefusal(t, []string{"--to", "start"}, "merge in progress", "lyx fabric merge --abort")
}

func TestResetCmd_DetachedHeadRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-detached")
	fx.saveState(t, startedAt(fx.base))
	gitkit.Git(t, fx.checkout, "checkout", "--detach")
	fx.wantRefusal(t, []string{"--to", "start"}, "git switch <task-branch>", "re-run `lyx webster reset --to start`")
}

func TestResetCmd_ParentBranchRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-parent")
	fx.saveState(t, startedAt(fx.base))
	fx.cli.parentBranch = func() (string, error) { return fx.branch, nil }
	fx.wantRefusal(t, []string{"--to", "start"}, "parent branch", "git switch <task-branch>")
}

func TestResetCmd_NonPairBranchRefusedByFabric(t *testing.T) {
	fx := newResetFixture(t, "rst-nonpair")
	fx.saveState(t, startedAt(fx.base))
	gitkit.Git(t, fx.checkout, "checkout", "-b", "not-the-pair-branch")
	fx.wantRefusal(t, []string{"--to", "start"}, "reset --to start refused")
}

func TestResetCmd_NoRecordedTargetRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-notarget")
	fx.saveState(t, &websterengine.State{MasterSessionID: "master-session", Batches: map[int]*websterengine.BatchState{}})
	fx.wantRefusal(t, []string{"--to", "start"}, "no batch recorded a start commit", "lyx webster run --fresh")
	fx.wantRefusal(t, []string{"--to", "pre-fix"}, "no pre-fix head", "run `lyx webster run`")
}

func TestResetCmd_MissingTargetCommitRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-missing")
	bogus := strings.Repeat("ab", 20)
	fx.saveState(t, startedAt(bogus))
	fx.wantRefusal(t, []string{"--to", "start"}, bogus, "not in this repository", "fetch the task branch")
}

func TestResetCmd_TargetNotAncestorRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-notancestor")
	gitkit.Git(t, fx.checkout, "checkout", "-b", "rst-notancestor-side", fx.base)
	side := gitkit.CommitFile(t, fx.checkout, "side.txt", "side", "side")
	gitkit.Git(t, fx.checkout, "checkout", fx.branch)
	st := startedAt(side)
	st.PreFixHead = side
	fx.saveState(t, st)
	fx.wantRefusal(t, []string{"--to", "start"}, "not an ancestor of HEAD", "lyx webster run --fresh")
	fx.wantRefusal(t, []string{"--to", "pre-fix"}, "not an ancestor of HEAD", "lyx webster reset --to start")
}

func TestResetCmd_DirtyPathTheRunDidNotWriteRefuses(t *testing.T) {
	fx := newResetFixture(t, "rst-foreign")
	fx.saveState(t, startedAt(fx.base))
	if err := os.WriteFile(filepath.Join(fx.checkout, "later.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	fx.wantRefusal(t, []string{"--to", "start"}, "later.txt", "git checkout -- <path>", "re-run `lyx webster reset --to start`")
}

func TestResetCmd_StandaloneRefusesNamingGitResetKeep(t *testing.T) {
	fx := newResetFixture(t, "rst-standalone")
	fx.saveState(t, startedAt(fx.base))
	fx.cli.openFabric = nil
	fx.cli.parentBranch = nil
	fx.wantRefusal(t, []string{"--to", "start"}, "standalone", "git reset --keep "+fx.base)
}
