//go:build integration && !windows

// teardown_integration_test.go drives the composite on a real hub pair with a real reed session and a stub driver strand.

package pairteardown_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// livePair is a hub pair with a reed session in its task worktree and a stub driver strand in it.
type livePair struct {
	h     *hubforge.Hub
	td    *pairteardown.Teardown
	slug  string
	eng   *reedengine.Engine
	guid  string
	tmux  string
	sess  string
	socky string
}

// newLivePair adds a pair and starts a session with a stub strand named by the driver role.
func newLivePair(t *testing.T, slug string) *livePair {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	hubforge.AddPair(t, h, slug)

	td, err := pairteardown.New(h.Location)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	eng, err := td.ReedEngineForTest(slug)
	if err != nil {
		t.Fatalf("ReedEngineForTest: %v", err)
	}
	t.Cleanup(func() { _, _ = eng.Down() })

	strand, err := eng.AddStrand(reedengine.AddSpec{Role: "driver", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	return &livePair{
		h: h, td: td, slug: slug, eng: eng, guid: strand.GUID, tmux: cfg.Tmux,
		sess:  reedengine.SessionName(fabricengine.WorktreePath(h.Location, slug)),
		socky: reedengine.ServerName(h.Path),
	}
}

// sessionUp reports whether the pair's session exists on the hub's socket.
func (p *livePair) sessionUp(t *testing.T) bool {
	t.Helper()
	sessions, err := reedengine.ListSessions(p.tmux, p.socky)
	if err != nil {
		return false
	}
	for _, s := range sessions {
		if s == p.sess {
			return true
		}
	}
	return false
}

// requireStrandLive fails the test unless the stub driver strand is still live.
func (p *livePair) requireStrandLive(t *testing.T) {
	t.Helper()
	res, err := p.eng.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for _, s := range res.Strands {
		if s.GUID == p.guid && s.Live {
			return
		}
	}
	t.Fatalf("stub driver strand %s is not live: %+v", p.guid, res.Strands)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRun_TaskSideDirtinessRefusalLeavesSessionAndStrandsLive(t *testing.T) {
	p := newLivePair(t, "pt-dirty")
	writeFile(t, filepath.Join(p.h.PairWarpWorktree(p.slug), "dirty.txt"), "uncommitted\n")

	_, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("Run error = %v, want the task-side dirtiness refusal", err)
	}
	if !p.sessionUp(t) {
		t.Error("session is gone after a probe refusal")
	}
	p.requireStrandLive(t)
}

func TestRun_SiblingChangesOutsidePathspecLeaveEverythingUntouched(t *testing.T) {
	p := newLivePair(t, "pt-sibling")
	sibling := p.h.PairWeftSibling(p.slug)
	writeFile(t, filepath.Join(sibling, "stray.txt"), "not a record\n")
	tip := gitkit.RevParse(t, sibling, "HEAD")

	_, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug})
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Fatalf("Run error = %v, want ErrPairSiblingDirty", err)
	}
	if !p.sessionUp(t) {
		t.Error("session is gone after a probe refusal")
	}
	p.requireStrandLive(t)
	if got := gitkit.RevParse(t, sibling, "HEAD"); got != tip {
		t.Errorf("sibling branch tip = %s, want unchanged %s", got, tip)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("sibling worktree gone after a refusal: %v", err)
	}
}

func TestRun_EndsTheSessionAndRemovesThePair(t *testing.T) {
	p := newLivePair(t, "pt-run")

	res, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Session.Ended || !res.Session.DriverWasLive {
		t.Errorf("session result = %+v, want Ended and DriverWasLive for a zero-wait run over a live driver", res.Session)
	}
	if p.sessionUp(t) {
		t.Error("session is still up after Run")
	}
	if _, err := os.Stat(fabricengine.WorktreePath(p.h.Location, p.slug)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("<hub>/<slug> after Run: stat err = %v, want not-exist", err)
	}
}

func TestRun_TaskWorktreeRemovedByHandEndsTheSessionByName(t *testing.T) {
	p := newLivePair(t, "pt-gone")
	if _, err := gitexec.Run([]string{"worktree", "remove", "--force", p.h.PairWarpWorktree(p.slug)}, p.h.PrimeWorktree()); err != nil {
		t.Fatalf("remove the task worktree by hand: %v", err)
	}
	if !p.sessionUp(t) {
		t.Fatal("session is not up before Run")
	}

	res, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug, QuietWait: pairteardown.RemoveQuietWait, RefuseWhenBusy: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Session.Ended {
		t.Errorf("session result = %+v, want Ended", res.Session)
	}
	if p.sessionUp(t) {
		t.Error("session is still up after Run")
	}
	if _, err := os.Stat(fabricengine.WorktreePath(p.h.Location, p.slug)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("<hub>/<slug> after Run: stat err = %v, want not-exist", err)
	}
}

func TestRun_WaitsForRetiringDriverThenArchivesTheCommittedReport(t *testing.T) {
	p := newLivePair(t, "pt-wait")
	p.td.SetIntervalForTest(50 * time.Millisecond)

	sibling := p.h.PairWeftSibling(p.slug)
	rel := "_lyx/shed/" + p.slug + "/drive-reports/stop.md"
	writeFile(t, filepath.Join(sibling, filepath.FromSlash(rel)), "stopped\n")
	gitkit.MustRun(t, sibling, "git", "add", rel)
	gitkit.MustRun(t, sibling, "git", "commit", "-m", "stop report")

	var marked bool
	realSleep := p.td.SleepForTest()
	p.td.SetSleepForTest(func(ctx context.Context, d time.Duration) error {
		if !marked {
			marked = true
			if err := p.eng.MarkRetiring(p.guid, true); err != nil {
				t.Errorf("MarkRetiring: %v", err)
			}
		}
		return realSleep(ctx, d)
	})

	res, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug, QuietWait: 10 * time.Second, RefuseWhenBusy: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !marked {
		t.Error("the quiet wait never slept, so the driver was never marked retiring during it")
	}
	if res.Session.DriverWasLive {
		t.Error("DriverWasLive = true, want false once the driver was marked retiring")
	}
	if res.Removal.ArchiveTag == "" {
		t.Fatal("Removal.ArchiveTag is empty")
	}
	out, err := gitexec.Run([]string{"show", "refs/tags/" + res.Removal.ArchiveTag + ":" + rel}, p.h.WeftBare)
	if err != nil {
		t.Fatalf("show the report in the archive tag: %v", err)
	}
	if out != "stopped\n" {
		t.Errorf("archived report = %q, want %q", out, "stopped\n")
	}
}
