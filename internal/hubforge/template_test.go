// template_test.go tests registered templates.
// It covers concurrent builds by key, what a copy holds and the packing of a built template.
// It also covers the failures a build, its refused TB calls, its upstream tip check and a shared key raise.

package hubforge

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rendezvous holds each arriving goroutine until a set number of them have arrived.
type rendezvous struct {
	remaining atomic.Int32
	all       chan struct{}
}

func newRendezvous(count int32) *rendezvous {
	r := &rendezvous{all: make(chan struct{})}
	r.remaining.Store(count)
	return r
}

// arrive waits until every goroutine has arrived.
// It fails tb when the others do not arrive within a minute, which only a build queued behind this one causes.
func (r *rendezvous) arrive(tb testing.TB) {
	if r.remaining.Add(-1) == 0 {
		close(r.all)
	}
	select {
	case <-r.all:
	case <-time.After(time.Minute):
		tb.Fatalf("the other key's build never ran beside this one; builds of different keys queue")
	}
}

// Two registrations whose builds each wait for the other, so they finish only when built at once.
var (
	concurrentBuilds      = newRendezvous(2)
	concurrentBuildCounts [2]atomic.Int32
	concurrentKeys        = [2]*Registration{
		{Key: "test-concurrent-a", Base: Shape{Anchor: "."}, Build: func(tb testing.TB, _ *Hub) {
			concurrentBuildCounts[0].Add(1)
			concurrentBuilds.arrive(tb)
		}},
		{Key: "test-concurrent-b", Base: Shape{Anchor: "backend"}, Build: func(tb testing.TB, _ *Hub) {
			concurrentBuildCounts[1].Add(1)
			concurrentBuilds.arrive(tb)
		}},
	}
)

func TestCopyRegistered_KeysBuildConcurrentlyOnceEach(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const callersPerKey = 3
	var wg sync.WaitGroup
	for _, registration := range concurrentKeys {
		for range callersPerKey {
			wg.Add(1)
			go func() {
				defer wg.Done()
				CopyRegistered(t, registration)
			}()
		}
	}
	wg.Wait()

	for i, registration := range concurrentKeys {
		if got := concurrentBuildCounts[i].Load(); got != 1 {
			t.Errorf("builds of %q = %d; want 1", registration.Key, got)
		}
	}
}

// buildStateSlug is the pair the build-state registration's build adds.
const buildStateSlug = "built-pair"

// buildStateRegistration commits and pushes a file on the board and adds a pair, over the base shape's one pair.
var buildStateRegistration = &Registration{
	Key:  "test-build-state",
	Base: Shape{Anchor: ".", Pairs: []string{TemplatePairSlug}},
	Build: func(tb testing.TB, h *Hub) {
		gitkit.CommitFile(tb, h.BoardDir(), "built.txt", "built\n", "hubforge test: build commit")
		gitkit.Git(tb, h.BoardDir(), "push", "--quiet", "origin", "HEAD")
		AddPair(tb, h, buildStateSlug)
	},
}

// TestCopyRegistered_CopyHoldsTheBuildAndIsolatesWrites asserts that a copy holds the build's commit and every pair.
// Its board's history is as long as the template's, with the relocated binding folded into its HEAD and pushed.
// It writes and pushes into that copy, and asserts the template is unchanged and a later copy holds none of the write.
func TestCopyRegistered_CopyHoldsTheBuildAndIsolatesWrites(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := CopyRegistered(t, buildStateRegistration)
	template := templateFor(t, buildStateRegistration)

	if got, err := os.ReadFile(filepath.Join(h.BoardDir(), "built.txt")); err != nil || string(got) != "built\n" {
		t.Errorf("the copy's board built.txt = %q, %v; want %q", got, err, "built\n")
	}
	if subjects := gitkit.Git(t, h.BoardDir(), "log", "--format=%s"); !strings.Contains(subjects, "hubforge test: build commit") {
		t.Errorf("the copy's board log = %q; want the build's commit in it", subjects)
	}
	copyHistory := gitkit.RevListCount(t, h.BoardDir(), "HEAD")
	if built := gitkit.RevListCount(t, template.hub.BoardDir(), "HEAD"); copyHistory != built {
		t.Errorf("the copy's board history holds %d commits; want the template's %d", copyHistory, built)
	}
	assertBindingFoldedAndPushed(t, h)
	for _, slug := range []string{TemplatePairSlug, buildStateSlug} {
		if _, err := os.Stat(h.PairCodeWorktree(slug)); err != nil {
			t.Errorf("pair %s's code worktree in the copy: %v", slug, err)
		}
		if _, err := os.Stat(h.PairRecordsSibling(slug)); err != nil {
			t.Errorf("pair %s's records sibling in the copy: %v", slug, err)
		}
	}

	gitkit.CommitFile(t, h.BoardDir(), "written.txt", "written\n", "hubforge test: write to the copy")
	gitkit.Git(t, h.BoardDir(), "push", "--quiet", "origin", "HEAD")

	if current, err := fingerprint(template.root); err != nil || current != template.fingerprint {
		t.Errorf("the template's fingerprint after a write to its copy = %s, %v; want the build-time %s", current, err, template.fingerprint)
	}
	later := CopyRegistered(t, buildStateRegistration)
	if _, err := os.Stat(filepath.Join(later.BoardDir(), "written.txt")); !os.IsNotExist(err) {
		t.Errorf("a later copy's board holds the earlier copy's write (stat err = %v); want it absent", err)
	}
}

// assertBindingFoldedAndPushed asserts that the copy's board is clean and its HEAD is its upstream.
// The HEAD is also its branch's tip in the copy's records bare, and the tracking ref's newest reflog entry names it.
func assertBindingFoldedAndPushed(t *testing.T, h *Hub) {
	t.Helper()

	board := h.BoardDir()
	if porcelain := gitkit.GitStatusPorcelain(t, board); porcelain != "" {
		t.Errorf("git status in the copy's board = %q; want a clean checkout, as a fresh hub's is", porcelain)
	}
	head := gitkit.RevParse(t, board, "HEAD")
	if upstream := gitkit.RevParse(t, board, "@{upstream}"); upstream != head {
		t.Errorf("the copy's board upstream = %s; want its HEAD %s, with the relocated binding pushed", upstream, head)
	}
	branch := gitkit.CurrentBranch(t, board)
	if tip := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+branch); tip != head {
		t.Errorf("the tip of %s in the copy's records bare = %s; want the board's HEAD %s", branch, tip, head)
	}
	trackingRef := gitkit.Git(t, board, "rev-parse", "--symbolic-full-name", "@{upstream}")
	if newest := gitkit.Git(t, board, "reflog", "show", "-n", "1", "--format=%H", trackingRef); newest != head {
		t.Errorf("the newest reflog entry of %s = %q; want the board's HEAD %s", trackingRef, newest, head)
	}
}

// TestSharedHub_PackedTemplateHoldsNoLooseObject takes a shape's template no other test takes.
// It asserts that the SharedHub sees no change since the build,
// and that no repository of the template holds a loose object or a loose branch ref.
func TestSharedHub_PackedTemplateHoldsNoLooseObject(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	shape := Shape{Anchor: "backend", Pairs: []string{TemplatePairSlug}}
	SharedHub(t, shape)
	root := templateFor(t, shapeRegistration(t, shape)).root

	repositories := 0
	var loose, looseRefs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slashed := "/" + filepath.ToSlash(relative)
		isBranchRef := strings.Contains(slashed, "/refs/heads/") || strings.Contains(slashed, "/refs/remotes/")
		isRemoteHead := strings.Contains(slashed, "/refs/remotes/") && d.Name() == "HEAD"
		// A remote's HEAD is a symbolic ref, which stays loose after pack-refs.
		if d.Type().IsRegular() && isBranchRef && !isRemoteHead && !strings.Contains(slashed, "/logs/refs/") {
			looseRefs = append(looseRefs, path)
		}
		parent := filepath.Dir(path)
		if d.IsDir() && d.Name() == "objects" {
			if _, err := os.Stat(filepath.Join(parent, "HEAD")); err == nil {
				repositories++
			}
			return nil
		}
		fanout := filepath.Base(parent)
		isFanout := len(fanout) == 2 && strings.Trim(fanout, "0123456789abcdef") == ""
		if d.Type().IsRegular() && isFanout && filepath.Base(filepath.Dir(parent)) == "objects" {
			loose = append(loose, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the template %s: %v", root, err)
	}
	if repositories == 0 {
		t.Fatalf("found no repository under the template %s; want its bares and clones", root)
	}
	if len(loose) != 0 {
		t.Errorf("loose objects in the template = %v; want every object packed at build time", loose)
	}
	if len(looseRefs) != 0 {
		t.Errorf("loose branch refs in the template = %v; want every ref packed at build time", looseRefs)
	}
}

// failingBuild is a registration whose build fails, with the count of its builds.
type failingBuild struct {
	name         string
	registration *Registration
	wantFirst    string
	builds       *atomic.Int32
}

// newFailingBuild registers build under the key test-failing-<name>, counting each time it runs.
func newFailingBuild(name, wantFirst string, build func(tb testing.TB, h *Hub)) failingBuild {
	builds := &atomic.Int32{}
	return failingBuild{
		name:      name,
		wantFirst: wantFirst,
		builds:    builds,
		registration: &Registration{Key: "test-failing-" + name, Base: Shape{Anchor: "."}, Build: func(tb testing.TB, h *Hub) {
			builds.Add(1)
			build(tb, h)
		}},
	}
}

// failingBuilds holds one registration per way a build fails.
var failingBuilds = []failingBuild{
	newFailingBuild("Fatal", "the build broke", func(tb testing.TB, _ *Hub) { tb.Fatalf("the build broke") }),
	newFailingBuild("Errorf", "the build erred", func(tb testing.TB, _ *Hub) { tb.Errorf("the build erred") }),
	newFailingBuild("Skip", "TB's Skip:", func(tb testing.TB, _ *Hub) { tb.Skip("x") }),
	newFailingBuild("Skipf", "TB's Skipf:", func(tb testing.TB, _ *Hub) { tb.Skipf("x") }),
	newFailingBuild("SkipNow", "TB's SkipNow:", func(tb testing.TB, _ *Hub) { tb.SkipNow() }),
	newFailingBuild("Cleanup", "Cleanup", func(tb testing.TB, _ *Hub) { tb.Cleanup(func() {}) }),
	newFailingBuild("Setenv", "Setenv", func(tb testing.TB, _ *Hub) { tb.Setenv("HUBFORGE_TEMPLATE_TEST", "1") }),
	newFailingBuild("TempDir", "TempDir", func(tb testing.TB, _ *Hub) { tb.TempDir() }),
	newFailingBuild("Chdir", "Chdir", func(tb testing.TB, h *Hub) { tb.Chdir(h.Path) }),
	newFailingBuild("ArtifactDir", "ArtifactDir", func(tb testing.TB, _ *Hub) { tb.ArtifactDir() }),
	newFailingBuild("BoardAheadOfTheBare", "is not the tip", func(tb testing.TB, h *Hub) {
		gitkit.CommitFile(tb, h.BoardDir(), "unpushed.txt", "unpushed\n", "hubforge test: unpushed")
	}),
	newFailingBuild("BarePushedFromElsewhere", "is not the tip", func(tb testing.TB, h *Hub) {
		clone := filepath.Join(filepath.Dir(h.Container), "pusher")
		branch := gitkit.CurrentBranch(tb, h.BoardDir())
		gitkit.Git(tb, filepath.Dir(h.Container), "clone", "--quiet", "--branch", branch, h.RecordsBare, clone)
		gitkit.CommitFile(tb, clone, "elsewhere.txt", "elsewhere\n", "hubforge test: pushed from elsewhere")
		gitkit.Git(tb, clone, "push", "--quiet", "origin", "HEAD")
	}),
}

// TestCopyRegistered_FailedBuildFailsEveryCallerNamingTheKey runs one row per way a build fails.
// It asserts that the first caller fails naming the key and the cause.
// It also asserts that a later caller fails naming the key without a rebuild.
func TestCopyRegistered_FailedBuildFailsEveryCallerNamingTheKey(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, tt := range failingBuilds {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key := tt.registration.Key
			first := &recordingTB{TB: t}
			first.run(func() { CopyRegistered(first, tt.registration) })
			if !first.failedNaming(key) || !first.failedNaming(tt.wantFirst) {
				t.Errorf("the first caller's failures = %v; want one naming %q and %q", first.failures, key, tt.wantFirst)
			}

			later := &recordingTB{TB: t}
			later.run(func() { CopyRegistered(later, tt.registration) })
			if !later.failedNaming(key) {
				t.Errorf("a later caller's failures = %v; want one naming %q", later.failures, key)
			}
			if got := tt.builds.Load(); got != 1 {
				t.Errorf("builds of %q = %d; want 1", key, got)
			}
		})
	}
}

// Two different registrations under one key.
var (
	sharedKeyFirst  = &Registration{Key: "test-shared-key", Base: Shape{Anchor: "."}}
	sharedKeySecond = &Registration{Key: "test-shared-key", Base: Shape{Anchor: "."}}
)

func TestCopyRegistered_SecondRegistrationOfAKeyFailsNamingIt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	CopyRegistered(t, sharedKeyFirst)

	second := &recordingTB{TB: t}
	second.run(func() { CopyRegistered(second, sharedKeySecond) })
	if !second.failedNaming(sharedKeySecond.Key) {
		t.Errorf("the second registration's failures = %v; want one naming %q", second.failures, sharedKeySecond.Key)
	}
}
