//go:build tmux && !windows

// teardown_integration_test.go drives the composite on real hub pairs with real reed sessions and a stub driver strand, as one ordered scenario over one hub and its tmux server.

package pairteardown_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
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

// addLivePair adds a pair to h and starts a session with a stub strand named by the driver role.
// Each pair gets its own Teardown and reed engine; the session ends when the calling test finishes.
func addLivePair(t *testing.T, h *hubforge.Hub, tmuxBinary, slug string) *livePair {
	t.Helper()

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
		h: h, td: td, slug: slug, eng: eng, guid: strand.GUID, tmux: tmuxBinary,
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

// startAndReap starts cmd and reaps it in the background, returning a channel closed once it has exited.
func startAndReap(t *testing.T, cmd *exec.Cmd) <-chan struct{} {
	t.Helper()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", cmd.Args, err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return done
}

// waitUntil polls cond until it holds, failing the test with what after a few seconds.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForPID waits for the file at path to hold a pid and returns it.
func waitForPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	waitUntil(t, "the pid file "+path, func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil
	})
	return pid
}

// processGone reports whether pid names no process, or one that has exited and awaits its reaper.
func processGone(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	_, afterName, _ := strings.Cut(string(data), ") ")
	return strings.HasPrefix(afterName, "Z")
}

// TestRun_TeardownScenario runs the composite's real-substrate checks over one hub.
// Each step adds its own pair and session, so the refusal steps cannot disturb the steps that remove a pair, and a session a step leaves up ends with that step.
// Steps run serially in this order: the step that ends the hub's last session by name comes last, because it asserts that the server's socket file goes with it.
// The scenario calls t.Parallel as a whole; no step does, because they share the one hub and its tmux server.
func TestRun_TeardownScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"TaskSideDirtinessRefusalLeavesSessionAndStrandsLive", func(t *testing.T) {
			p := addLivePair(t, h, cfg.Tmux, "pt-dirty")
			writeFile(t, filepath.Join(p.h.PairCodeWorktree(p.slug), "dirty.txt"), "uncommitted\n")

			_, err := p.td.Run(context.Background(), pairteardown.Request{Slug: p.slug})
			if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
				t.Fatalf("Run error = %v, want the task-side dirtiness refusal", err)
			}
			if !p.sessionUp(t) {
				t.Error("session is gone after a probe refusal")
			}
			p.requireStrandLive(t)
		}},
		{"SiblingChangesOutsidePathspecLeaveEverythingUntouched", func(t *testing.T) {
			p := addLivePair(t, h, cfg.Tmux, "pt-sibling")
			sibling := p.h.PairRecordsSibling(p.slug)
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
		}},
		{"EndsTheSessionAndRemovesThePair", func(t *testing.T) {
			p := addLivePair(t, h, cfg.Tmux, "pt-run")

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
		}},
		{"WaitsForRetiringDriverThenArchivesTheCommittedReport", func(t *testing.T) {
			p := addLivePair(t, h, cfg.Tmux, "pt-wait")
			p.td.SetIntervalForTest(50 * time.Millisecond)

			sibling := p.h.PairRecordsSibling(p.slug)
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
			out, err := gitexec.Run([]string{"show", "refs/tags/" + res.Removal.ArchiveTag + ":" + rel}, p.h.RecordsBare)
			if err != nil {
				t.Fatalf("show the report in the archive tag: %v", err)
			}
			if out != "stopped\n" {
				t.Errorf("archived report = %q, want %q", out, "stopped\n")
			}
		}},
		{"LiveLoopAndItsStepTreeDieBeforeTheSessionEnds", func(t *testing.T) {
			if _, err := exec.LookPath("flock"); err != nil {
				t.Skipf("flock not found, so no stand-in loop can hold the loop lock: %v", err)
			}
			p := addLivePair(t, h, cfg.Tmux, "pt-loop")
			task, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(p.h.Location, p.slug))
			if err != nil {
				t.Fatalf("ResolveWorktree: %v", err)
			}
			pidPath := shedrun.LoopPIDFile(task, shedrun.SelfRunID)
			lockPath := shedrun.LoopLock(task, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				t.Fatalf("create the steps directory: %v", err)
			}

			// The stand-in loop is a process holding the loop lock; the stand-in child is its own process group holding a grandchild.
			// The shell locks its own descriptor and execs sleep, so the process that holds the lock keeps the recorded pid.
			loop := exec.Command("sh", "-c", `exec 9>"$1"; flock 9 && exec sleep 300`, "sh", lockPath)
			grandchildFile := filepath.Join(t.TempDir(), "grandchild.pid")
			child := exec.Command("sh", "-c", `sleep 300 & echo $! > "$1"; wait`, "sh", grandchildFile)
			child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			loopDone, childDone := startAndReap(t, loop), startAndReap(t, child)
			t.Cleanup(func() { _ = loop.Process.Kill(); _ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL) })

			grandchild := waitForPID(t, grandchildFile)
			waitUntil(t, "the stand-in loop to hold the loop lock", func() bool {
				held, ok, err := lock.TryAcquireWriteLock(lockPath)
				if err == nil && ok {
					_ = held.Release()
				}
				return err == nil && !ok
			})
			loopStart, _ := proc.StartTime(loop.Process.Pid)
			childStart, _ := proc.StartTime(child.Process.Pid)
			record := shedverbs.LoopPIDRecord{
				LoopID: "stand-in",
				Loop:   proc.TreeRecord{PID: loop.Process.Pid, StartTime: loopStart},
				Child:  proc.TreeRecord{PID: child.Process.Pid, PGID: child.Process.Pid, StartTime: childStart},
			}
			if err := shedverbs.WriteLoopPIDRecord(pidPath, record); err != nil {
				t.Fatalf("write the stand-in loop's pid record: %v", err)
			}

			if _, err := p.td.EndSession(context.Background(), pairteardown.Request{Slug: p.slug}); err != nil {
				t.Fatalf("EndSession: %v", err)
			}
			for name, done := range map[string]<-chan struct{}{"loop": loopDone, "child": childDone} {
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Errorf("the stand-in %s is still running after EndSession", name)
				}
			}
			waitUntil(t, "the grandchild to die", func() bool { return processGone(grandchild) })
			if p.sessionUp(t) {
				t.Error("session is still up after EndSession")
			}
			held, ok, err := lock.TryAcquireWriteLock(shedrun.RunLock(task, shedrun.SelfRunID))
			if err != nil || !ok {
				t.Fatalf("the run lock after EndSession: ok = %v, err = %v; want it free", ok, err)
			}
			_ = held.Release()
		}},
		{"TaskWorktreeRemovedByHandEndsTheSessionByName", func(t *testing.T) {
			// Runs last: it ends the only live session on the hub's server, so the server's socket file must go.
			p := addLivePair(t, h, cfg.Tmux, "pt-gone")
			if _, err := gitexec.Run([]string{"worktree", "remove", "--force", p.h.PairCodeWorktree(p.slug)}, p.h.PrimeWorktree()); err != nil {
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
			// Ending the last session by name kills the hub's server, and its socket file must go with it.
			socketPath := filepath.Join(os.Getenv("TMUX_TMPDIR"), "tmux-"+strconv.Itoa(os.Getuid()), p.socky)
			if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("socket file %s after ending the last session by name: lstat err = %v, want not-exist", socketPath, err)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
