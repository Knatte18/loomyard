// run_fixture_test.go holds the fakes and the fixture TestRun drives Run over.
// A real shuttle runner over the shuttlefake reed and engine sits behind a scripted StrandRunner.
// Beside it sit an in-memory git, a scripted verify and a scripted verify fixer.

package weaverengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// testGateConfig returns a gate config holding the `card`, `batch` and `webster` keys.
func testGateConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}, "tier1": {About: "integration"}},
		Gates: map[string]gateslot.GateKey{
			"card":    {Tiers: []int{0}, Scope: "card"},
			"batch":   {Tiers: []int{0, 1}, Scope: "card"},
			"webster": {Tiers: []int{0, 1}, Scope: "all"},
		},
	}
}

// fakeClock is a clock whose time advances only when Sleep is called.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeGit answers the git questions Run asks from an in-memory commit chain.
// Every other method of websterengine.Git is nil and panics when called.
type fakeGit struct {
	websterengine.Git
	mu      sync.Mutex
	counter int
	head    string
	parents map[string][]string
	dirty   []string
	// merging is what MergeInProgress answers.
	merging bool
	// rejection is what MergeRejection answers for every merge.
	rejection string
	log       []runplan.CommitLogEntry
}

func newFakeGit() *fakeGit {
	g := &fakeGit{parents: map[string][]string{}}
	g.head = g.mint()
	return g
}

// mint returns a new 40-digit commit id.
func (g *fakeGit) mint() string {
	g.counter++
	return fmt.Sprintf("%040x", g.counter)
}

// commit adds a commit on top of HEAD and returns it.
func (g *fakeGit) commit(subject string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	sha := g.mint()
	g.parents[sha] = []string{g.head}
	g.head = sha
	g.log = append(g.log, runplan.CommitLogEntry{SHA: sha, Subject: subject})
	return sha
}

// merge adds a two-parent commit on top of HEAD and returns it.
func (g *fakeGit) merge() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	sha := g.mint()
	g.parents[sha] = []string{g.head, g.mint()}
	g.head = sha
	return sha
}

func (g *fakeGit) HeadSHA(string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.head, nil
}

func (g *fakeGit) MergeInProgress(string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.merging, nil
}

func (g *fakeGit) DirtyPaths(string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.dirty...), nil
}

func (g *fakeGit) CommitParents(_, commit string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.parents[commit], nil
}

func (g *fakeGit) MergeRejection(string, string, []string, websterengine.ParentBranchFunc) string {
	return g.rejection
}

func (g *fakeGit) CommitsNamedBy(_, prefix string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var named []string
	for sha := range g.parents {
		if strings.HasPrefix(sha, prefix) {
			named = append(named, sha)
		}
	}
	return named, nil
}

func (g *fakeGit) CommitLog(string, string, string) ([]runplan.CommitLogEntry, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]runplan.CommitLogEntry(nil), g.log...), nil
}

// fakeIndex is a planindex.Index that finds nothing wrong; it embeds nil for every method Run never calls.
type fakeIndex struct {
	planindex.Index
	// notDone is what DoneChecks reports.
	notDone []planindex.Finding
	// notDoneTarget, when set, limits notDone to the asked cards that still name this Create target.
	notDoneTarget string
	// removed is what the delta reports as removed later targets.
	removed []planindex.Finding
	// removedOnlyWithLaterCards limits removed to a batch that has a card still to be built after it.
	removedOnlyWithLaterCards bool
	// rewriteRenames makes the drift pass rewrite each reported rename into every card not yet built.
	rewriteRenames bool
	// dispatch is what the dispatch re-resolution reports.
	dispatch []planindex.Finding
}

func (i fakeIndex) ValidateDispatch(*planparser.Plan, string, []planparser.Card, []planparser.Card) ([]planindex.Finding, error) {
	return i.dispatch, nil
}

// DoneChecks reports notDone.
// When notDoneTarget is set, it reports notDone only while a Create group of the asked cards still names that target.
func (i fakeIndex) DoneChecks(_ *planparser.Plan, cards []planparser.Card, _ string) ([]planindex.Finding, error) {
	if i.notDoneTarget == "" {
		return i.notDone, nil
	}
	for _, c := range cards {
		for _, g := range c.TargetGroups {
			if g.Type == planparser.CardTypeCreate && slices.Contains(g.Refs, i.notDoneTarget) {
				return i.notDone, nil
			}
		}
	}
	return nil, nil
}

func (fakeIndex) RebindResigns(*planparser.Plan, []planparser.Card, string) ([]planindex.Finding, error) {
	return nil, nil
}

func (i fakeIndex) Delta(string, string, string) (planindex.Delta, error) {
	return fakeDelta{removed: i.removed, removedOnlyWithLaterCards: i.removedOnlyWithLaterCards, rewriteRenames: i.rewriteRenames}, nil
}

type fakeDelta struct {
	planindex.Delta
	removed                   []planindex.Finding
	removedOnlyWithLaterCards bool
	rewriteRenames            bool
}

func (fakeDelta) BindHandles(*planparser.Plan, string, []planparser.Card) ([]planindex.Finding, []planindex.BindOutcome, error) {
	return nil, nil, nil
}

func (fakeDelta) ScopeGuard(*planparser.Plan, []planparser.Card) []planindex.Finding { return nil }

func (d fakeDelta) RemovedTargets(plan *planparser.Plan, completed, own []planparser.Card, _ []planindex.Rename) []planindex.Finding {
	if d.removedOnlyWithLaterCards {
		later := slices.ContainsFunc(plan.Cards, func(card planparser.Card) bool {
			built := func(c planparser.Card) bool { return c.Number == card.Number }
			return !slices.ContainsFunc(completed, built) && !slices.ContainsFunc(own, built)
		})
		if !later {
			return nil
		}
	}
	return d.removed
}

func (d fakeDelta) DetectDrift(plan *planparser.Plan, completed []planparser.Card, reported []planindex.Rename, planDir, _, _, _ string) ([]planindex.Finding, error) {
	if !d.rewriteRenames {
		return nil, nil
	}
	for _, card := range plan.Cards {
		if slices.ContainsFunc(completed, func(c planparser.Card) bool { return c.Number == card.Number }) {
			continue
		}
		path := filepath.Join(planDir, runplan.CardID(card)+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		text := string(data)
		for _, rename := range reported {
			text = strings.ReplaceAll(text, rename.Old, rename.New)
		}
		if text == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// chunkBatcher forms batches of at most size cards.
type chunkBatcher struct{ size int }

func (chunkBatcher) Name() string { return "chunk" }

func (b chunkBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, _ batcher.SizeSource, _ int, _ batcher.StartBase) ([]batcher.Batch, error) {
	var batches []batcher.Batch
	for start := 0; start < len(cards); start += b.size {
		batches = append(batches, batcher.Batch{Cards: cards[start:min(start+b.size, len(cards))]})
	}
	return batches, nil
}

// attemptScript scripts one batch attempt of the scripted runner.
type attemptScript struct {
	// startErr makes the start fail.
	startErr error
	// noReport leaves the report file unwritten.
	noReport bool
	// report is the report written, with HeadSHA left empty for HEAD at the time it is written.
	report runplan.Report
	// commit makes the attempt commit before it writes its report.
	commit bool
	// rawReport writes these bytes as the report instead.
	rawReport string
	// failGate makes the named gate entry fail terminally.
	failGate string
	// dieAfterStart makes reed lose the strand, so the wait fails.
	dieAfterStart bool
	// dirty is the dirty paths the worktree holds when the attempt ends.
	dirty []string
	// abbreviate writes the report's head_sha as its first twelve digits.
	abbreviate bool
	// mergeOnTop adds a clean parent merge on top of the report's head.
	mergeOnTop bool
	// onGate runs once, when the strand's first gate entry is evaluated, which is while the run waits on the strand.
	onGate func()
}

// scriptedRunner is a StrandRunner over a real shuttle runner on fake reed and engine.
// It starts each strand with the entries' closures replaced by scripted ones.
// It writes the scripted report after the start.
type scriptedRunner struct {
	inner *shuttleengine.Runner
	reed  *shuttlefake.Reed
	git   *fakeGit

	mu      sync.Mutex
	scripts []attemptScript
	specs   []shuttleengine.Spec
	gates   []shuttleengine.GateSpec
	stops   []string
	stopErr error
	// onStart is called with the index of each start, before the script is read.
	onStart func(index int)
	// probe answers ProbeGated; nil finds no run.
	probe func(spec shuttleengine.Spec) (*shuttleengine.Run, bool, error)
	// probed holds the spec of every probe.
	probed []shuttleengine.Spec
}

var _ weaverengine.StrandRunner = (*scriptedRunner)(nil)

// EngineName returns the engine the choice names, else "claude", which the seeded stencils ship a directive for.
func (r *scriptedRunner) EngineName(choice shuttleengine.ModelChoice) string {
	if choice.Engine != "" {
		return choice.Engine
	}
	return "claude"
}

func (r *scriptedRunner) StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (*shuttleengine.Run, error) {
	r.reed.StatusErr = nil
	r.mu.Lock()
	index := len(r.specs)
	if r.onStart != nil {
		r.onStart(index)
	}
	r.specs = append(r.specs, spec)
	r.gates = append(r.gates, gate)
	var script attemptScript
	if index < len(r.scripts) {
		script = r.scripts[index]
	}
	r.mu.Unlock()
	if script.startErr != nil {
		return nil, script.startErr
	}

	scripted := make(shuttleengine.GateSpec, len(gate))
	copy(scripted, gate)
	var onGateOnce sync.Once
	for i := range scripted {
		failing := scripted[i].Name == script.failGate
		scripted[i].Gate = func() (shuttleengine.GateResult, error) {
			if script.onGate != nil {
				onGateOnce.Do(script.onGate)
			}
			if failing {
				return shuttleengine.GateResult{Terminal: true, Findings: "the gate found a failure"}, nil
			}
			return shuttleengine.GateResult{Passed: true}, nil
		}
	}
	run, err := r.inner.StartGated(spec, scripted)
	if err != nil {
		return nil, err
	}
	if script.commit {
		r.git.commit("the attempt's work")
	}
	r.git.mu.Lock()
	r.git.dirty = script.dirty
	r.git.mu.Unlock()
	if script.dieAfterStart {
		r.reed.StatusErr = fmt.Errorf("reed lost the strand")
		return run, nil
	}
	if script.noReport {
		return run, nil
	}
	if script.rawReport != "" {
		return run, os.WriteFile(spec.OutputFiles[0], []byte(script.rawReport), 0o644)
	}
	report := script.report
	if report.Status == "" {
		report.Status = runplan.ReportStatusOK
	}
	if report.HeadSHA == "" {
		report.HeadSHA, _ = r.git.HeadSHA("")
	}
	if script.mergeOnTop {
		r.git.merge()
	}
	if script.abbreviate {
		report.HeadSHA = report.HeadSHA[:12]
	}
	return run, runplan.WriteReport(spec.OutputFiles[0], &report)
}

func (r *scriptedRunner) ProbeGated(spec shuttleengine.Spec, _ shuttleengine.GateSpec) (*shuttleengine.Run, bool, error) {
	r.mu.Lock()
	r.probed = append(r.probed, spec)
	probe := r.probe
	r.mu.Unlock()
	if probe == nil {
		return nil, false, nil
	}
	return probe(spec)
}

func (r *scriptedRunner) StopStrand(guid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops = append(r.stops, guid)
	return r.stopErr
}

// gateNames returns the entry names of the spec the nth start was handed.
func (r *scriptedRunner) gateNames(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var names []string
	for _, entry := range r.gates[n] {
		names = append(names, entry.Name)
	}
	return names
}

// fixerScript scripts one verify fixer.
type fixerScript struct {
	// dead ends the fixer dead, and otherwise it ends done.
	dead bool
	// warnings are the warnings its wait returns.
	warnings []string
	// commit is the subject of a commit the fixer makes, empty for none.
	commit string
}

// fakeFixer is a gatefixer.Fixer over scripts.
type fakeFixer struct {
	git *fakeGit

	mu      sync.Mutex
	scripts []fixerScript
	specs   []gatefixer.Spec
	saved   []gatefixer.Record
	// attached counts the calls that found a live record and spawned nothing.
	attached int
}

var _ gatefixer.Fixer = (*fakeFixer)(nil)

func (f *fakeFixer) SpawnOrAttach(store gatefixer.Store, spec gatefixer.Spec) (*gatefixer.Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if live, _ := store.Live(); live != nil {
		f.attached++
		return live, false, nil
	}
	f.specs = append(f.specs, spec)
	rec := &gatefixer.Record{StrandGUID: fmt.Sprintf("fixer-%d", len(f.specs)), SpawnedAt: time.Unix(0, 0).UTC().Format(time.RFC3339), ReplyPath: spec.ReplyPath}
	return rec, true, store.Save(rec)
}

func (f *fakeFixer) Await(rec *gatefixer.Record, _, _ time.Duration) (*gatefixer.Result, error) {
	f.mu.Lock()
	index := len(f.specs) - 1
	var script fixerScript
	if index >= 0 && index < len(f.scripts) {
		script = f.scripts[index]
	}
	f.mu.Unlock()
	if script.commit != "" {
		f.git.commit(script.commit)
	}
	result := &gatefixer.Result{Status: gatefixer.StatusDone, Warnings: script.warnings}
	if script.dead {
		result.Status, result.DeadReason = gatefixer.StatusDead, gatefixer.DeadReasonDied
	}
	return result, nil
}

func (f *fakeFixer) Stop(*gatefixer.Record) error { return nil }

// verifyScript scripts one call of the verify seam.
type verifyScript struct {
	status verifytree.Status
	// failing names the test the failed run's log reports as failing.
	failing  string
	timedOut bool
	crash    bool
	dirty    []string
}

// scriptedVerify is a weaverengine.VerifyFunc over scripts; a call past the scripts passes.
type scriptedVerify struct {
	dir string

	mu      sync.Mutex
	scripts []verifyScript
	calls   []string
}

func (v *scriptedVerify) run(_ verifytree.Site, command string) (verifytree.Result, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	index := len(v.calls)
	v.calls = append(v.calls, command)
	if index >= len(v.scripts) {
		return verifytree.Result{Status: verifytree.StatusPassed}, nil
	}
	script := v.scripts[index]
	if script.status == "" {
		script.status = verifytree.StatusFailed
	}
	res := verifytree.Result{Status: script.status, ExitCode: 1, TimedOut: script.timedOut, RunnerCrash: script.crash, Dirty: script.dirty, Detail: "scripted"}
	if script.status == verifytree.StatusFailed {
		res.Log = filepath.Join(v.dir, fmt.Sprintf("verify-%d.log", index))
		body := fmt.Sprintf("--- FAIL: %s (0.00s)\n    broken\nFAIL\nFAIL\texample.test/pkg\t0.01s\n", script.failing)
		if err := os.WriteFile(res.Log, []byte(body), 0o644); err != nil {
			return verifytree.Result{}, err
		}
	}
	return res, nil
}

// runFixture is Run's deps over the fakes, with the paths of the run.
type runFixture struct {
	t       *testing.T
	deps    weaverengine.RunDeps
	anchor  string
	git     *fakeGit
	index   *fakeIndex
	runner  *scriptedRunner
	fixer   *fakeFixer
	verify  *scriptedVerify
	reed    *shuttlefake.Reed
	mu      sync.Mutex
	commits []string
	// commitErr makes the commit seam fail when it returns non-nil for the message.
	commitErr func(msg string) error
}

// commitMessages returns every message the commit seam was handed.
func (f *runFixture) commitMessages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commits...)
}

// newRunFixture writes a plan of the given card slugs, numbered from one, and wires Run's deps over fakes.
// The batcher forms batches of chunk cards.
func newRunFixture(t *testing.T, chunk int, slugs ...string) *runFixture {
	t.Helper()
	anchor := t.TempDir()
	geom := weaverengine.Geometry{
		AnchorRoot:   anchor,
		WorktreeRoot: anchor,
		PaneCwd:      anchor,
		RunDir:       weaverengine.Dir(anchor),
		ReportsDir:   weaverengine.ReportsDir(anchor),
		PromptsDir:   weaverengine.PromptsDir(anchor),
		ScratchDir:   weaverengine.ScratchDir(anchor),
		StencilsDir:  stencilkit.Seed(t),
		PlanDir:      planparser.PlanDir(anchor),
		VerifyDir:    filepath.Join(anchor, "verify"),
		GateConfig:   func() (gateslot.Config, error) { return testGateConfig(), nil },

		DecisionRecordPath: filepath.Join(anchor, "decision.md"),
	}
	cards := make([]plankit.Card, len(slugs))
	for i, slug := range slugs {
		cards[i] = plankit.Card{Number: i + 1, Slug: slug, Groups: []plankit.Group{{Label: "Create", Targets: []string{"internal/x/" + slug + ".go"}}}}
	}
	plankit.Write(t, geom.PlanDir, plankit.Plan{Approved: true, Language: "none", Framing: "A test plan.", Cards: cards})

	git := newFakeGit()
	index := &fakeIndex{}
	geom.Git = git
	geom.Index = index

	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30, PollIntervalMS: 1000, LivenessEveryNPolls: 1, Agents: shuttlefake.Agents(t)}
	inner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(engine), anchor, anchor, shuttleCfg)
	inner.SetClock(&fakeClock{now: time.Unix(0, 0)})

	fx := &runFixture{
		t: t, anchor: anchor, git: git, index: index, reed: reed,
		runner: &scriptedRunner{inner: inner, reed: reed, git: git},
		fixer:  &fakeFixer{git: git},
		verify: &scriptedVerify{dir: t.TempDir()},
	}
	fx.deps = weaverengine.RunDeps{
		Geom:    geom,
		Config:  weaverengine.Config{Ladder: []string{"a", "b", "c"}, FixerTimeoutMin: 7, VerifyGate: "webster"},
		Ladder:  []shuttleengine.ModelChoice{{Model: "model-a"}, {Model: "model-b"}, {Model: "model-c"}},
		Batcher: chunkBatcher{size: chunk},
		Runner:  fx.runner,
		Fixer:   fx.fixer,
		Reed:    reed,
		Verify:  fx.verify.run,
		CommitRecords: func(msg string) error {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			fx.commits = append(fx.commits, msg)
			if fx.commitErr != nil {
				return fx.commitErr(msg)
			}
			return nil
		},
		RecordsHead: func() (string, error) { return "records-head", nil },
		Clock:       &fakeClock{now: time.Unix(0, 0)},
	}
	return fx
}

// loadState loads the state Run left in the fixture's run directory.
func (f *runFixture) loadState() *weaverengine.State {
	f.t.Helper()
	st, err := weaverengine.LoadState(f.deps.Geom.RunDir, f.deps.Geom.ScratchDir)
	if err != nil || st == nil {
		f.t.Fatalf("LoadState() = %v, %v; want the recorded state", st, err)
	}
	return st
}
