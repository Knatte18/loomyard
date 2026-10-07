// runlevel_test.go exercises Run end to end (Tier 1 — see
// docs/benchmarks/running-tests.md): a temp directory over a fakeGit backs
// WorktreeRoot and a real on-disk plan directory backs PlanDir (so
// ParsePlan/Validate/the batchifier/fingerprint all run for real, against
// the real identity batchifier — newRunFixture injects it into
// RunDeps.Batcher directly, since Run itself does no config I/O and needs no
// _lyx/ tree on WorktreeRoot), while the Master spawn itself is a local,
// fully-scripted fake (MasterStarter/
// MasterHandle) whose Wait method can carry an onWait
// side effect that writes outcome.yaml/summary.md the instant before it
// returns — modeling the real ordering (Master writes its two contract
// files DURING the run Wait blocks on). FindRun's cross-process resolution
// is satisfied by hand-seeding a run.json under the fixture's own shuttle
// run-dir root, mirroring what a real *shuttleengine.Runner.Start would
// have produced. Its helpers are package-local (the
// internal and external test packages deliberately do not share a
// test-helper package, mirroring recoverbatch_test.go/recordbatch_test.go's
// own precedent), except for the shared seedPlanDir/mustFingerprint/writeWorktreeFile
// helpers already defined in beginbatch_test.go and gitfake_test.go.

package websterengine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// runFakeHandle is a hermetic websterengine.MasterHandle double: Wait runs
// the caller-scripted onWait side effect (if any) — modeling Master writing
// its contract files DURING the call Wait blocks on — then returns the
// scripted Result/error.
type runFakeHandle struct {
	strandGUID string
	result     shuttleengine.Result
	waitErr    error
	onWait     func()
}

func (h *runFakeHandle) StrandGUID() string { return h.strandGUID }

func (h *runFakeHandle) Wait() (shuttleengine.Result, error) {
	if h.onWait != nil {
		h.onWait()
	}
	return h.result, h.waitErr
}

var _ websterengine.MasterHandle = (*runFakeHandle)(nil)

// runFakeStarter is a hermetic websterengine.MasterStarter double: it
// records every spec and gate StartMaster was called with and hands back the
// caller-scripted handle, or startErr when the caller wants to prove a step
// never reaches the spawn at all.
type runFakeStarter struct {
	mu         sync.Mutex
	startCalls []shuttleengine.Spec
	gateCalls  []shuttleengine.GateSpec
	handle     websterengine.MasterHandle
	startErr   error
}

func (s *runFakeStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls = append(s.startCalls, spec)
	s.gateCalls = append(s.gateCalls, gate)
	if s.startErr != nil {
		return nil, s.startErr
	}
	return s.handle, nil
}

func (s *runFakeStarter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.startCalls)
}

var _ websterengine.MasterStarter = (*runFakeStarter)(nil)

// seedRunPlanDir writes a syntactically complete, validation-clean
// plan-format plan with numCards cards into a fresh temp plan directory:
// each card carries a format-4 Create group whose single target is the card's own
// new-file path (so path-missing never fires — a Create group's targets stay
// exempt from on-disk existence checking exactly as Creates: entries were).
// The overview carries NO plan-level "## verify:" section — deliberately,
// so the verify gate passes without running anything for every fixture built on this helper;
// a test that needs one calls appendIntegrationVerify against an already-seeded plan dir.
// numCards == 0 yields a "## Card Index" section with no entries at all,
// which ParsePlan's own parseCardIndex refuses loud ("no card index entries
// found") — the vehicle for the zero-batch refusal test, which under the
// flat model is actually the batchifier-derived-zero-batches refusal (an
// empty Cards list, if it ever parsed, would batchify to zero batches too).
func seedRunPlanDir(t *testing.T, numCards int) string {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, runPlan(1, numCards))
	return dir
}

// runPlan returns a validation-clean plan of numCards cards numbered from first, each a Create of its own new file.
func runPlan(first, numCards int) plankit.Plan {
	p := plankit.Plan{Approved: true, Framing: "Framing."}
	if first != 1 {
		p.FirstCard = first
	}
	for n := first; n < first+numCards; n++ {
		slug := fmt.Sprintf("batch%d", n)
		p.Cards = append(p.Cards, plankit.Card{
			Number:  n,
			Slug:    slug,
			Summary: fmt.Sprintf("placeholder card %d", n),
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{fmt.Sprintf("internal/%s/new.go", slug)}}},
			Intent:  "placeholder card.",
		})
	}
	return p
}

// seedShuttleRunState hand-seeds a run.json under runDirRoot naming
// strandGUID/sessionID, satisfying shuttleengine.FindRun's cross-process
// scan the way a real *shuttleengine.Runner.Start would — without needing to
// actually drive one (Run's own fake MasterStarter never touches the real
// run-dir machinery).
func seedShuttleRunState(t *testing.T, runDirRoot, strandGUID, sessionID string) {
	t.Helper()
	runDir := filepath.Join(runDirRoot, "fake-run-"+strandGUID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	rs := shuttleengine.RunState{
		RunID:      "fake-run-" + strandGUID,
		StrandGUID: strandGUID,
		SessionID:  sessionID,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		t.Fatalf("marshal run state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), data, 0o644); err != nil {
		t.Fatalf("write run.json: %v", err)
	}
}

// runFixture is a fully-wired set of Run dependencies: a temp directory
// holding base.txt as WorktreeRoot over a fakeGit at one commit, a real on-disk plan directory, a fake reed/engine,
// and a fake Starter a test scripts per case.
type runFixture struct {
	Deps           websterengine.RunDeps
	Reed           *shuttlefake.Reed
	Starter        *runFakeStarter
	Git            *fakeGit
	Worktree       string
	PlanDir        string
	ShuttleRunRoot string
}

func newRunFixture(t *testing.T, numCards int) *runFixture {
	t.Helper()
	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	fx := newRunFixtureOver(t, numCards, worktree, git)
	fx.Git = git
	return fx
}

// newRunFixtureOver builds the fixture over worktree, answering git questions from git.
// A nil git means the real repository at worktree.
func newRunFixtureOver(t *testing.T, numCards int, worktree string, git websterengine.Git) *runFixture {
	t.Helper()

	_, index := indexOver(git)

	planDir := seedRunPlanDir(t, numCards)

	// Run never registers a strand itself, so a stray AddStrand fails loud.
	reed := &shuttlefake.Reed{AddErr: errors.New("AddStrand is not used by Run's own path")}
	starter := &runFakeStarter{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before Run reaches RenderMasterPrompt.
	seedHubStencils(t, hubPath)
	shuttleRunRoot := t.TempDir()
	shuttleCfg := shuttleengine.Config{RunDir: shuttleRunRoot, RunTimeoutMin: 60, StartupTimeoutS: 30}

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{"effort": "high"}},
	}

	// Select("") is the right call here rather than Active: the fixture's
	// WorktreeRoot is a bare scratch git repo with no _lyx/ tree, and the
	// point of the runlevel-call-site decision is that Run needs no config
	// tree.
	activeBatcher, err := batcher.Select("")
	if err != nil {
		t.Fatalf("batcher.Select(\"\") error = %v", err)
	}

	deps := websterengine.RunDeps{
		Starter:    starter,
		Reed:       reed,
		Engine:     &shuttlefake.Engine{},
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Batcher:    activeBatcher,
		Config: websterengine.Config{
			SelfFixCap:         2,
			MasterTimeoutMin:   480,
			VerifyGateAttempts: 3,
		},
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			VerifyDir:    t.TempDir(),
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   t.TempDir(),
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			PlanDir:      planDir,
			Git:          git,
			Index:        index,
		},
		RefMatcher: websterengine.NeverMatches{},
	}

	return &runFixture{Deps: deps, Reed: reed, Starter: starter, Worktree: worktree, PlanDir: planDir, ShuttleRunRoot: shuttleRunRoot}
}

// appendIntegrationVerify appends a plan-level "## verify:" section to the overview of the already-seeded plan dir at planDir,
// so a fixture built without one can exercise the plan-level verify gate.
func appendIntegrationVerify(t *testing.T, planDir, verify string) {
	t.Helper()
	path := filepath.Join(planDir, "00-overview.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overview fixture: %v", err)
	}
	data = append(data, []byte("\n## verify:\n\n"+verify+"\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write overview fixture with verify: %v", err)
	}
}

// addCardUses rewrites an already-seeded card file under planDir — one of seedRunPlanDir's own
// "%02d-batch%d.md" files — to carry a "**Uses:**" field naming ref, modelled on
// appendIntegrationVerify: read the file, splice the field in, write it
// back. Inserted ahead of the card's own "**Intent:**" line, which every seedRunPlanDir card
// carries.
// A path-shaped ref this points at another card's Create target is already satisfied by
// planparser.Validate's own createTargetsUnion check within the same plan, but a caller that wants
// the ref to also resolve on disk (mirroring a real cross-card file dependency) must still create it
// under the fixture's own worktree — this helper only edits the plan-format text.
func addCardUses(t *testing.T, planDir string, cardNumber int, ref string) {
	t.Helper()
	slug := fmt.Sprintf("batch%d", cardNumber)
	path := filepath.Join(planDir, fmt.Sprintf("%02d-%s.md", cardNumber, slug))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read card fixture %s: %v", path, err)
	}
	const marker = "\n**Intent:**"
	body := string(data)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("card fixture %s carries no **Intent:** marker to splice **Uses:** ahead of", path)
	}
	usesBlock := fmt.Sprintf("\n**Uses:**\n- `%s`\n", ref)
	newBody := body[:idx] + usesBlock + body[idx:]
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		t.Fatalf("write card fixture with Uses: %v", err)
	}
}

// addCardCreateTarget rewrites an already-seeded card file under planDir to add target as an
// additional bullet under the card's existing "**Create:**" group, ahead of its own
// seedRunPlanDir-written path target -- so a test can add a glyph-shaped Create target alongside
// the plain-path one every seedRunPlanDir card already carries, without disturbing it.
func addCardCreateTarget(t *testing.T, planDir string, cardNumber int, target string) {
	t.Helper()
	slug := fmt.Sprintf("batch%d", cardNumber)
	path := filepath.Join(planDir, fmt.Sprintf("%02d-%s.md", cardNumber, slug))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read card fixture %s: %v", path, err)
	}
	const marker = "**Create:**\n"
	body := string(data)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("card fixture %s carries no **Create:** marker to splice an extra target under", path)
	}
	insertAt := idx + len(marker)
	newBody := body[:insertAt] + fmt.Sprintf("- `%s`\n", target) + body[insertAt:]
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		t.Fatalf("write card fixture with an extra Create target: %v", err)
	}
}

// seedMatchingState saves st into fx's webster dir after stamping its
// PlanFingerprint to match fx's own on-disk plan directory (and defaulting
// its Batches map when nil), so Run's own fingerprint gate passes and the
// pre-seeded state survives into the run unmodified.
func seedMatchingState(t *testing.T, fx *runFixture, st *websterengine.State) {
	t.Helper()
	st.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	if err := websterengine.RestampPlanBaseline(st, fx.PlanDir, fx.Deps.Geom.WebsterDir); err != nil {
		t.Fatalf("stamp plan file hashes: %v", err)
	}
	if st.Batches == nil {
		st.Batches = map[int]*websterengine.BatchState{}
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("seed matching state: %v", err)
	}
}

// loadRunState loads the state Run left in fx's webster dir.
func loadRunState(t *testing.T, fx *runFixture) *websterengine.State {
	t.Helper()
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want recorded state", st, err)
	}
	return st
}

// readSummary reads the summary.md Run left in fx's webster dir.
func readSummary(t *testing.T, fx *runFixture) string {
	t.Helper()
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	return string(summary)
}

// doneBatch is a recorded terminal done batch of the fork kind, begun by session.
func doneBatch(slug, session string) *websterengine.BatchState {
	return &websterengine.BatchState{Slug: slug, Kind: "fork", Terminal: true, Status: "done", SessionID: session}
}

// forkReports returns count clean fork reports, one transcript each.
func forkReports(count int) []shuttleengine.ForkReport {
	reports := make([]shuttleengine.ForkReport, count)
	for i := range reports {
		reports[i] = shuttleengine.ForkReport{TranscriptPath: fmt.Sprintf("/transcripts/fork%d.jsonl", i+1), ReportReturned: true}
	}
	return reports
}

// writeContractFiles writes the two files Merriam's last action writes: outcome.yaml carrying outcome and summary.md carrying summary.
func writeContractFiles(t *testing.T, fx *runFixture, outcome, summary string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
		t.Fatalf("write outcome.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// captureLogs redirects the logger into a buffer for the rest of the test.
// The logger's output is process-global, so a test calling it never runs in parallel.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &logs
}

// TestRun_RefusesBeforeSpawn asserts each refusal Run raises ahead of the Master spawn: the typed
// error or message it carries, the way forward it names, that the Starter is never reached, and —
// where the card has a way forward to take — that taking it lets the re-run reach the Master.
func TestRun_RefusesBeforeSpawn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		cards int
		// setup edits the fixture to provoke the refusal and returns the step that takes the way
		// forward; nil means the case has no way forward to take.
		setup          func(t *testing.T, fx *runFixture) (takeWayForward func())
		errIs          error
		msgContains    []string
		msgNotContains []string
		wayForward     []string
		// reachesStarter marks a refusal raised by the Starter itself.
		reachesStarter bool
		check          func(t *testing.T, fx *runFixture, err error)
	}{
		{
			name:  "another run holds run.lock",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				if err := os.MkdirAll(fx.Deps.Geom.ScratchDir, 0o755); err != nil {
					t.Fatalf("mkdir webster scratch dir: %v", err)
				}
				held, err := lock.AcquireWriteLock(filepath.Join(fx.Deps.Geom.ScratchDir, "run.lock"))
				if err != nil {
					t.Fatalf("acquire run.lock: %v", err)
				}
				t.Cleanup(func() { held.Release() })
				return func() { held.Release() }
			},
			errIs:      websterengine.ErrRunBusy,
			wayForward: []string{"lyx webster status"},
		},
		{
			name:  "no batchifier was populated",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				fx.Deps.Batcher = nil
				return nil
			},
			errIs: websterengine.ErrNilBatcher,
		},
		{
			name:  "a plan parsing to zero cards",
			cards: 0,
		},
		{
			name:  "the batchifier derives zero batches",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				realBatcher := fx.Deps.Batcher
				fx.Deps.Batcher = emptyBatcher{}
				return func() { fx.Deps.Batcher = realBatcher }
			},
			wayForward: []string{"fix the plan's cards", "lyx webster rebaseline --card", "lyx webster run"},
		},
		{
			name:  "a blocking glyph finding",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				writeWorktreeFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Foo() {}\n")
				cardPath := filepath.Join(fx.PlanDir, "01-batch1.md")
				original, err := os.ReadFile(cardPath)
				if err != nil {
					t.Fatalf("read card: %v", err)
				}
				addCardUses(t, fx.PlanDir, 1, "sub#Missing")
				return func() {
					// The refused run already recorded the edited plan; fixing the card is a
					// further edit, so the next run refuses it as foreign until the operator
					// rebaselines, which the message names.
					if err := os.WriteFile(cardPath, original, 0o644); err != nil {
						t.Fatalf("fix card: %v", err)
					}
					askingMaster(t, fx, "validated")
					if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); !errors.Is(err, websterengine.ErrFingerprintMismatch) {
						t.Fatalf("Run() after the plan fix error = %v; want errors.Is(err, ErrFingerprintMismatch) until rebaselined", err)
					}
					rebaselineOnDisk(t, fx, 1)
				}
			},
			msgContains: []string{"glyph-not-found"},
			wayForward:  []string{"fix the named cards", "lyx webster rebaseline --card", "lyx webster run"},
		},
		{
			name:  "quarry is unavailable",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				worktree := fx.Deps.Geom.WorktreeRoot
				fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")
				return func() { fx.Deps.Geom.WorktreeRoot = worktree }
			},
			msgContains: []string{"quarry"},
			wayForward:  []string{"transient", "lyx webster run", "quarry"},
		},
		{
			name:  "the plan is not approved",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				overview := filepath.Join(fx.PlanDir, "00-overview.md")
				data, err := os.ReadFile(overview)
				if err != nil {
					t.Fatalf("read overview: %v", err)
				}
				if err := os.WriteFile(overview, []byte(strings.Replace(string(data), "approved: true", "approved: false", 1)), 0o644); err != nil {
					t.Fatalf("write overview: %v", err)
				}
				return func() {
					if err := os.WriteFile(overview, data, 0o644); err != nil {
						t.Fatalf("approve plan: %v", err)
					}
				}
			},
			msgContains: []string{"not approved"},
			wayForward:  []string{"approve the plan", "lyx webster run"},
		},
		{
			// A refusal leaves a pending pause request untouched: only a run that passes every gate clears it.
			name:  "a stale fingerprint without --fresh",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				st := &websterengine.State{PlanFingerprint: "stale-fingerprint", Batches: map[int]*websterengine.BatchState{}}
				if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
					t.Fatalf("seed stale state: %v", err)
				}
				if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
					t.Fatalf("RequestPause() error = %v", err)
				}
				return nil
			},
			errIs:      websterengine.ErrFingerprintMismatch,
			wayForward: []string{"lyx webster rebaseline", "lyx webster run --fresh"},
			check: func(t *testing.T, fx *runFixture, err error) {
				if !websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
					t.Error("pause flag cleared on a refused run; want it left intact")
				}
			},
		},
		{
			name:  "an edited card names the card to rebaseline",
			cards: 2,
			setup: func(t *testing.T, fx *runFixture) func() {
				seedMatchingState(t, fx, &websterengine.State{})
				addCardUses(t, fx.PlanDir, 2, "base.txt")
				return nil
			},
			errIs:      websterengine.ErrFingerprintMismatch,
			wayForward: []string{"lyx webster rebaseline --card 02", "lyx webster run --fresh"},
		},
		{
			name:  "an edited overview names no card to rebaseline",
			cards: 2,
			setup: func(t *testing.T, fx *runFixture) func() {
				seedMatchingState(t, fx, &websterengine.State{})
				overview := filepath.Join(fx.PlanDir, "00-overview.md")
				data, err := os.ReadFile(overview)
				if err != nil {
					t.Fatalf("read overview: %v", err)
				}
				if err := os.WriteFile(overview, []byte(strings.Replace(string(data), "Framing.", "Framing, edited.", 1)), 0o644); err != nil {
					t.Fatalf("edit overview: %v", err)
				}
				return nil
			},
			errIs:          websterengine.ErrFingerprintMismatch,
			wayForward:     []string{"--fresh"},
			msgNotContains: []string{"--card"},
		},
		{
			// The validation error is forced by pointing WorktreeRoot at a path with no repository,
			// so planglyph's own openRepo fails; the save failure is forced by making the webster dir
			// read-only after the matching state has been seeded, which is the one directory
			// SaveState writes into. The resolve pass has by then already rewritten the plan on
			// disk, so a state.json still holding the pre-rewrite fingerprint would make the NEXT run
			// refuse this run's own edit as a foreign one: both failures must be reported.
			name:  "a validation error and a failed re-baseline save are both reported",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				seedMatchingState(t, fx, &websterengine.State{})
				fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "no-such-tree")
				websterDir := fx.Deps.Geom.WebsterDir
				if err := os.Chmod(websterDir, 0o555); err != nil {
					t.Fatalf("chmod webster dir read-only: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(websterDir, 0o755) })
				return nil
			},
			errIs:       planglyph.ErrQuarryUnavailable,
			msgContains: []string{"re-baseline"},
		},
		{
			name:  "a gate that already names verify",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				fx.Deps.Gate = shuttleengine.GateSpec{{
					Name:     "verify",
					Gate:     func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil },
					Attempts: 1,
				}}
				return nil
			},
			wayForward: []string{"verify", "drop the"},
		},
		{
			name:  "the provider does not come up",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) func() {
				fx.Starter.startErr = errors.New("provider did not come up")
				return func() {}
			},
			reachesStarter: true,
			wayForward:     []string{"transient", "lyx webster run"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, tc.cards)
			var takeWayForward func()
			if tc.setup != nil {
				takeWayForward = tc.setup(t, fx)
			}

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			if err == nil {
				t.Fatal("Run() error = nil; want a refusal")
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Errorf("Run() error = %v; want errors.Is(err, %v)", err, tc.errIs)
			}
			for _, want := range tc.msgContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Run() error = %q; want it to contain %q", err, want)
				}
			}
			for _, unwanted := range tc.msgNotContains {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("Run() error = %q; want it free of %q", err, unwanted)
				}
			}
			if tc.wayForward != nil {
				requireWayForward(t, err, tc.wayForward...)
			}
			if !tc.reachesStarter && fx.Starter.callCount() != 0 {
				t.Errorf("Starter was reached (%d calls) before the refusal; want zero", fx.Starter.callCount())
			}
			if tc.check != nil {
				tc.check(t, fx, err)
			}

			if takeWayForward != nil {
				takeWayForward()
				askingMaster(t, fx, "recovered")
				_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
				requireReachedMaster(t, fx, err)
			}
		})
	}
}

// TestRun_EntryValidationReachesMaster asserts a run whose state or plan could trip the entry
// validation gate — a completed or begun Create card whose target already landed, a forthcoming
// Create target another card Uses, an informational-only findings set, a first init — still reaches the
// Master spawn exactly once, and that the first init records a hash for every plan file.
func TestRun_EntryValidationReachesMaster(t *testing.T) {
	t.Parallel()

	const startSHA = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name  string
		cards int
		setup func(t *testing.T, fx *runFixture)
		check func(t *testing.T, fx *runFixture)
	}{
		{
			// A resumed run whose state records batch 1 terminal, and whose batch-1 Create target
			// consequently exists on disk, must sail past the entry validation gate: the gate once
			// re-validated the WHOLE plan and refused the resume on create-already-exists.
			name:  "resume with a completed Create card",
			cards: 2,
			setup: func(t *testing.T, fx *runFixture) {
				writeWorktreeFile(t, fx.Worktree, "internal/batch1/new.go", "package batch1\n\nfunc Landed() {}\n")
				seedMatchingState(t, fx, &websterengine.State{
					RunGUID: "resume-run",
					Batches: map[int]*websterengine.BatchState{1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"}},
				})
			},
		},
		{
			// State records batch 1 begun but not terminal, with its Create target already committed.
			name:  "a begun, unrecorded batch resumes",
			cards: 2,
			setup: func(t *testing.T, fx *runFixture) {
				writeWorktreeFile(t, fx.Worktree, "internal/batch1/new.go", "package batch1\n\nfunc Landed() {}\n")
				seedMatchingState(t, fx, &websterengine.State{
					RunGUID: "resume-run",
					Batches: map[int]*websterengine.BatchState{1: {Slug: "batch1", Kind: "fork", StartSHA: startSHA}},
				})
			},
		},
		{
			// State records batch 1 begun but not terminal with nothing landed, and card 2 Uses
			// card 1's Create target, which does not exist yet.
			name:  "a forthcoming Create target another card Uses",
			cards: 2,
			setup: func(t *testing.T, fx *runFixture) {
				addCardUses(t, fx.PlanDir, 2, "internal/batch1/new.go")
				seedMatchingState(t, fx, &websterengine.State{
					RunGUID: "resume-run",
					Batches: map[int]*websterengine.BatchState{1: {Slug: "batch1", Kind: "fork", StartSHA: startSHA}},
				})
			},
		},
		{
			// An informational-only findings set — create-new-unit, on a Create target introducing a
			// brand-new package — is no refusal: severity decides the verdict.
			name:  "an informational-only findings set",
			cards: 1,
			setup: func(t *testing.T, fx *runFixture) {
				writeWorktreeFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Foo() {}\n")
				addCardCreateTarget(t, fx.PlanDir, 1, "newpkg#Bar")
			},
		},
		{
			name:  "a first init records a hash for every plan file",
			cards: 2,
			check: func(t *testing.T, fx *runFixture) {
				st := loadRunState(t, fx)
				for _, name := range []string{"00-overview.md", "01-batch1.md", "02-batch2.md"} {
					if st.PlanFileHashes[name] == "" {
						t.Errorf("PlanFileHashes = %v; want a hash for %s", st.PlanFileHashes, name)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, tc.cards)
			if tc.setup != nil {
				tc.setup(t, fx)
			}
			askingMaster(t, fx, "entry")

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			requireReachedMaster(t, fx, err)
			var target *websterengine.MasterAskingError
			if !errors.As(err, &target) {
				t.Errorf("Run() error = %v; want a *MasterAskingError", err)
			}
			if fx.Starter.callCount() != 1 {
				t.Errorf("Starter.callCount() = %d; want 1", fx.Starter.callCount())
			}
			if tc.check != nil {
				tc.check(t, fx)
			}
		})
	}
}

// TestRun_EntryHousekeeping asserts what Run does to stale files and recorded strands on entry,
// before the Master spawn: --fresh archives the stale state and reports and clears the prompts,
// stale contract files are archived, live recorded strands are stopped, and a cleanly absent strand
// is left alone.
func TestRun_EntryHousekeeping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		fresh bool
		// setup seeds the stale entry state and returns the checks to make once Run stopped before the spawn.
		setup func(t *testing.T, fx *runFixture) (verify func(t *testing.T))
	}{
		{
			name:  "--fresh archives the stale state and reports and clears the prompts",
			fresh: true,
			setup: func(t *testing.T, fx *runFixture) func(t *testing.T) {
				staleState := &websterengine.State{PlanFingerprint: "stale", Batches: map[int]*websterengine.BatchState{}}
				if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, staleState); err != nil {
					t.Fatalf("seed stale state: %v", err)
				}
				if err := os.MkdirAll(fx.Deps.Geom.ReportsDir, 0o755); err != nil {
					t.Fatalf("mkdir reports dir: %v", err)
				}
				reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, "01-batch1.yaml")
				if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
					t.Fatalf("seed stale report: %v", err)
				}
				if err := os.MkdirAll(fx.Deps.Geom.PromptsDir, 0o755); err != nil {
					t.Fatalf("mkdir prompts dir: %v", err)
				}
				promptPath := filepath.Join(fx.Deps.Geom.PromptsDir, "01-batch1.md")
				if err := os.WriteFile(promptPath, []byte("stale prompt\n"), 0o644); err != nil {
					t.Fatalf("seed stale prompt: %v", err)
				}

				return func(t *testing.T) {
					// The stale state.json is archived (renamed, content preserved) rather
					// than deleted; the live path is then reinitialized fresh by the same
					// --fresh sequence, so a state.json legitimately exists there again by
					// the time Run returns — it must no longer carry the stale fingerprint.
					stateFile := filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")
					archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "state-*.json"))
					if globErr != nil || len(archived) != 1 {
						t.Fatalf("archived state glob = %v, %v; want exactly 1", archived, globErr)
					}
					archivedData, err := os.ReadFile(archived[0])
					if err != nil {
						t.Fatalf("read archived state %s: %v", archived[0], err)
					}
					if !strings.Contains(string(archivedData), `"stale"`) {
						t.Errorf("archived state content = %q; want it to still carry the stale fingerprint", archivedData)
					}
					liveData, err := os.ReadFile(stateFile)
					if err != nil {
						t.Fatalf("read live state.json %s: %v", stateFile, err)
					}
					if strings.Contains(string(liveData), `"stale"`) {
						t.Errorf("live state.json still carries the stale fingerprint; want it reinitialized fresh")
					}

					if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
						t.Errorf("stale report still present at its original path; want the reports dir archived away wholesale")
					}
					archivedReportsDirs, globErr := filepath.Glob(fx.Deps.Geom.ReportsDir + "-*")
					if globErr != nil || len(archivedReportsDirs) != 1 {
						t.Fatalf("archived reports dir glob = %v, %v; want exactly 1", archivedReportsDirs, globErr)
					}
					if _, statErr := os.Stat(filepath.Join(archivedReportsDirs[0], "01-batch1.yaml")); statErr != nil {
						t.Errorf("archived reports dir missing the stale report: %v", statErr)
					}
					if info, statErr := os.Stat(fx.Deps.Geom.ReportsDir); statErr != nil || !info.IsDir() {
						t.Errorf("ReportsDir not recreated after --fresh archiving: %v", statErr)
					}

					if _, statErr := os.Stat(promptPath); !os.IsNotExist(statErr) {
						t.Errorf("rendered prompt file still present; want the prompts dir cleared (re-renderable, never archived)")
					}
				}
			},
		},
		{
			name: "stale outcome.yaml and summary.md are archived, never deleted",
			setup: func(t *testing.T, fx *runFixture) func(t *testing.T) {
				if err := os.MkdirAll(fx.Deps.Geom.WebsterDir, 0o755); err != nil {
					t.Fatalf("mkdir webster dir: %v", err)
				}
				outcomePath := filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml")
				summaryPath := filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md")
				if err := os.WriteFile(outcomePath, []byte("outcome: stuck\nstuck_reason: \"prior run\"\nbatches_done: 0\n"), 0o644); err != nil {
					t.Fatalf("seed stale outcome: %v", err)
				}
				if err := os.WriteFile(summaryPath, []byte("# Prior run\n\nStale.\n"), 0o644); err != nil {
					t.Fatalf("seed stale summary: %v", err)
				}

				return func(t *testing.T) {
					if _, statErr := os.Stat(outcomePath); !os.IsNotExist(statErr) {
						t.Errorf("stale outcome.yaml still present at its original path; want it archived away")
					}
					if _, statErr := os.Stat(summaryPath); !os.IsNotExist(statErr) {
						t.Errorf("stale summary.md still present at its original path; want it archived away")
					}
					if archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome-*.yaml")); globErr != nil || len(archived) != 1 {
						t.Errorf("archived outcome glob = %v, %v; want exactly 1", archived, globErr)
					}
					if archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "summary-*.md")); globErr != nil || len(archived) != 1 {
						t.Errorf("archived summary glob = %v, %v; want exactly 1", archived, globErr)
					}
				}
			},
		},
		{
			// A recorded, still-live Master strand and a recorded, non-terminal, still-live
			// recovery-batch strand are stopped; a recorded strand the reed no longer reports at
			// all is cleanly absent — already gone, nothing to stop.
			name: "live Master and recovery strands are stopped, an absent one is left alone",
			setup: func(t *testing.T, fx *runFixture) func(t *testing.T) {
				seedMatchingState(t, fx, &websterengine.State{
					MasterStrand: "prior-master-strand",
					Batches: map[int]*websterengine.BatchState{
						1: {Slug: "batch1", Kind: "recovery", Terminal: false, StrandGUID: "prior-recovery-strand"},
						2: {Slug: "batch2", Kind: "recovery", Terminal: false, StrandGUID: "absent-recovery-strand"},
					},
				})
				fx.Reed.Strands = []reedengine.StrandStatus{
					{GUID: "prior-master-strand", Live: true},
					{GUID: "prior-recovery-strand", Live: true},
					// "absent-recovery-strand" is deliberately absent from Status at all.
				}

				return func(t *testing.T) {
					wantRemoved := map[string]bool{"prior-master-strand": true, "prior-recovery-strand": true}
					for _, guid := range fx.Reed.RemovedGUIDs {
						if guid == "absent-recovery-strand" {
							t.Errorf("RemoveStrand called for a cleanly-absent strand %q; want it left untouched", guid)
						}
						delete(wantRemoved, guid)
					}
					if len(wantRemoved) != 0 {
						t.Errorf("RemoveStrand calls = %v; missing %v", fx.Reed.RemovedGUIDs, wantRemoved)
					}
				}
			},
		},
		{
			name: "a state recording no strand removes nothing",
			setup: func(t *testing.T, fx *runFixture) func(t *testing.T) {
				seedMatchingState(t, fx, &websterengine.State{})
				return func(t *testing.T) {
					if len(fx.Reed.RemovedGUIDs) != 0 {
						t.Errorf("RemoveStrand calls = %v; want none", fx.Reed.RemovedGUIDs)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1)
			verify := tc.setup(t, fx)
			fx.Starter.startErr = fmt.Errorf("stop before spawn")

			if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: tc.fresh}); err == nil {
				t.Fatalf("Run() error = nil; want the scripted starter error")
			}
			verify(t)
		})
	}
}

// TestRun_MasterSpawn asserts what Run hands the Master spawn and records around it: the strand role, model and skills, the awaited shell, the gate entries, the prompt (never written to a master.md), the strand and session identities, and the order of the batches the prompt lists.
func TestRun_MasterSpawn(t *testing.T) {
	t.Parallel()

	const (
		spawnStrand  = "master-strand-spawn"
		spawnSession = "master-session-spawn"
	)
	var gateInvoked atomic.Bool

	cases := []struct {
		name  string
		cards int
		// prepare edits the fixture before Run.
		prepare func(t *testing.T, fx *runFixture)
		// noRunState leaves the shuttle run state unseeded, so FindRun's session resolve fails after the spawn.
		noRunState bool
		check      func(t *testing.T, fx *runFixture)
	}{
		{
			// Merriam spawns under the strand role `webster` while the model still resolves from RoleMaster.
			name:  "the strand role, the model and the skills",
			cards: 1,
			check: func(t *testing.T, fx *runFixture) {
				spec := fx.Starter.startCalls[0]
				if spec.Role != websterengine.MerriamStrandRole || spec.Role == string(websterengine.RoleMaster) {
					t.Errorf("Spec.Role = %q; want %q, distinct from RoleMaster", spec.Role, websterengine.MerriamStrandRole)
				}
				if want := fx.Deps.Roles[websterengine.RoleMaster].Model; spec.Model != want {
					t.Errorf("Spec.Model = %q; want %q (RoleMaster's resolved model)", spec.Model, want)
				}
				if want := []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}; !slices.Equal(spec.Skills, want) {
					t.Errorf("Spec.Skills = %v; want %v", spec.Skills, want)
				}
			},
		},
		{
			name:  "the recover-batch shell is declared awaited",
			cards: 1,
			check: func(t *testing.T, fx *runFixture) {
				got := fx.Starter.startCalls[0].AwaitedShellPrefixes
				if !slices.Equal(got, []string{"lyx webster recover-batch"}) {
					t.Errorf("Spec.AwaitedShellPrefixes = %q; want the recover-batch prefix", got)
				}
			},
		},
		{
			// The strand and session identities the bracket verbs read are persisted BEFORE Run blocks on Wait.
			name:  "the strand and session identities are persisted",
			cards: 1,
			check: func(t *testing.T, fx *runFixture) {
				st := loadRunState(t, fx)
				if st.MasterStrand != spawnStrand {
					t.Errorf("State.MasterStrand = %q; want %q", st.MasterStrand, spawnStrand)
				}
				if st.MasterSessionID != spawnSession {
					t.Errorf("State.MasterSessionID = %q; want %q", st.MasterSessionID, spawnSession)
				}
			},
		},
		{
			// When FindRun fails AFTER Master's pane is live, state.json has already recorded
			// MasterStrand, so the next run's entry-time reclaim can find and stop the orphaned pane.
			name:       "the Master strand is persisted before the session resolve",
			cards:      1,
			noRunState: true,
			check: func(t *testing.T, fx *runFixture) {
				if st := loadRunState(t, fx); st.MasterStrand != spawnStrand {
					t.Errorf("State.MasterStrand = %q; want it persisted BEFORE the FindRun failure so the reclaim can find the orphan", st.MasterStrand)
				}
			},
		},
		{
			// The told gate is threaded, never rebuilt or dropped, and Run adds its own verify
			// entry; evaluating the closure is shuttleengine's own Wait's job.
			name:  "a told gate reaches StartMaster beside the verify entry",
			cards: 1,
			prepare: func(t *testing.T, fx *runFixture) {
				fx.Deps.Gate = shuttleengine.GateSpec{{
					Gate: func() (shuttleengine.GateResult, error) {
						gateInvoked.Store(true)
						return shuttleengine.GateResult{Passed: true}, nil
					},
					Attempts: 7,
				}}
			},
			check: func(t *testing.T, fx *runFixture) {
				if len(fx.Starter.gateCalls) != 1 {
					t.Fatalf("len(Starter.gateCalls) = %d; want 1", len(fx.Starter.gateCalls))
				}
				got := fx.Starter.gateCalls[0]
				if len(got) != 2 {
					t.Fatalf("StartMaster received %d gate entries; want the told one and the verify entry", len(got))
				}
				if got[0].Gate == nil {
					t.Error("StartMaster received a nil Gate; want the told closure")
				}
				if got[0].Attempts != 7 {
					t.Errorf("StartMaster received Attempts = %d; want 7", got[0].Attempts)
				}
				if got[1].Name != "verify" {
					t.Errorf("StartMaster's last gate entry = %q; want the verify entry Run adds", got[1].Name)
				}
				if gateInvoked.Load() {
					t.Error("the gate closure was invoked by Run; want it spent only by shuttle's own Wait")
				}
			},
		},
		{
			name:  "no told gate leaves only the verify entry",
			cards: 1,
			check: func(t *testing.T, fx *runFixture) {
				if len(fx.Starter.gateCalls) != 1 {
					t.Fatalf("len(Starter.gateCalls) = %d; want 1", len(fx.Starter.gateCalls))
				}
				got := fx.Starter.gateCalls[0]
				if len(got) != 1 || got[0].Name != "verify" {
					t.Fatalf("StartMaster received gate %+v; want exactly the verify entry", got)
				}
				if got[0].Attempts != fx.Deps.Config.VerifyGateAttempts {
					t.Errorf("verify entry Attempts = %d; want Config.VerifyGateAttempts %d", got[0].Attempts, fx.Deps.Config.VerifyGateAttempts)
				}
				if got[0].PassOnCap || got[0].MayHold {
					t.Errorf("verify entry = %+v; want a plain must-pass entry", got[0])
				}
			},
		},
		{
			name:  "the prompt is the rendered Master prompt and no master.md is written",
			cards: 1,
			check: func(t *testing.T, fx *runFixture) {
				prompt := fx.Starter.startCalls[0].Prompt
				if !strings.Contains(prompt, "01 — batch1") {
					t.Errorf("Spec.Prompt = %q; want the rendered Master prompt listing batch 01", prompt)
				}
				if _, err := os.Stat(filepath.Join(fx.Deps.Geom.PromptsDir, "master.md")); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("master.md stat err = %v; want it absent, since Run no longer writes it", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, tc.cards)
			if tc.prepare != nil {
				tc.prepare(t, fx)
			}
			fx.Starter.handle = &runFakeHandle{strandGUID: spawnStrand, waitErr: fmt.Errorf("stop after spawn")}
			if !tc.noRunState {
				seedShuttleRunState(t, fx.ShuttleRunRoot, spawnStrand, spawnSession)
			}

			if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err == nil {
				t.Fatalf("Run() error = nil; want the scripted wait error")
			}
			if len(fx.Starter.startCalls) != 1 {
				t.Fatalf("StartMaster calls = %d; want 1", len(fx.Starter.startCalls))
			}
			tc.check(t, fx)
		})
	}
}

// TestRun_MasterEndedEarly asserts each of the asking, died and timeout shuttle outcomes for
// Master's own spawn maps to its own typed error, carrying the SessionID and the kept RunDir and
// matching its sentinel via errors.Is, names the re-run as the way forward, and that a fresh Master
// resuming from state.json then finishes.
func TestRun_MasterEndedEarly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		outcome shuttleengine.Outcome
		check   func(t *testing.T, err error, wantSessionID, wantRunDir string)
	}{
		{
			outcome: shuttleengine.OutcomeAsking,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterAskingError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterAskingError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterAskingError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterAsking) {
					t.Error("errors.Is(err, ErrMasterAsking) = false; want true")
				}
			},
		},
		{
			outcome: shuttleengine.OutcomeDied,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterDiedError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterDiedError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterDiedError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterDied) {
					t.Error("errors.Is(err, ErrMasterDied) = false; want true")
				}
			},
		},
		{
			outcome: shuttleengine.OutcomeTimeout,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterTimeoutError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterTimeoutError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterTimeoutError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterTimeout) {
					t.Error("errors.Is(err, ErrMasterTimeout) = false; want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.outcome), func(t *testing.T) {
			t.Parallel()
			const (
				strand  = "master-strand-early"
				session = "master-session-early"
				runDir  = "/run/dir/early"
			)
			fx := newRunFixture(t, 1)
			seedMatchingState(t, fx, &websterengine.State{
				Batches: map[int]*websterengine.BatchState{1: doneBatch("batch1", session)},
			})
			fx.Starter.handle = &runFakeHandle{
				strandGUID: strand,
				result:     shuttleengine.Result{Outcome: tt.outcome, SessionID: session, RunDir: runDir, LastAssistantMessage: "why do you ask?"},
			}
			seedShuttleRunState(t, fx.ShuttleRunRoot, strand, session)

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			tt.check(t, err, session, runDir)
			requireWayForward(t, err, "lyx webster run", "re-step the Webster row", "resumes from state.json")

			runToDone(t, fx, strand, session, forkReports(1), 1)
		})
	}
}

// TestRun_DoneOutcome asserts what Run records and reports once Master writes outcome done over a
// clean or findings-bearing whole-session fork audit: the result, the warnings, the pending
// findings that demote the run to stuck, the summary sections and the fixer prompt.
func TestRun_DoneOutcome(t *testing.T) {
	t.Parallel()

	const fabricCommand = "cat FABRICREF/webster/state.json"
	cases := []struct {
		name    string
		cards   int
		session string
		// prepare edits the fixture before the state is seeded.
		prepare     func(t *testing.T, fx *runFixture)
		state       *websterengine.State
		audit       func(fx *runFixture) shuttleengine.ForkAudit
		batchesDone int
		check       func(t *testing.T, fx *runFixture, result websterengine.RunResult)
	}{
		{
			name:    "a valid summary and a clean audit populate the result",
			cards:   1,
			session: "master-session-done",
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			}},
			audit:       func(*runFixture) shuttleengine.ForkAudit { return shuttleengine.ForkAudit{Forks: forkReports(1)} },
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if result.Outcome != "done" {
					t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
				}
				if result.BatchesDone != 1 {
					t.Errorf("RunResult.BatchesDone = %d; want 1", result.BatchesDone)
				}
				if result.SummaryTitle != "Shipped" {
					t.Errorf("RunResult.SummaryTitle = %q; want %q", result.SummaryTitle, "Shipped")
				}
				if websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
					t.Error("pause flag present after a done outcome; want it cleared")
				}
			},
		},
		{
			name:    "a finding an earlier record-batch warned on is not warned again",
			cards:   3,
			session: "master-session-spawn",
			state: &websterengine.State{
				AuditDispositions: map[string]string{"master-session-spawn/parent:named-spawn:1": "warned"},
				Batches: map[int]*websterengine.BatchState{
					1: doneBatch("batch1", "master-session-spawn"),
					2: func() *websterengine.BatchState {
						batch := doneBatch("batch2", "master-session-spawn")
						batch.AuditWarnings = []websterengine.AuditWarning{{Identity: "master-session-spawn/parent:named-spawn:1", Class: "named-spawn", Detail: "master spawned a named agent"}}
						return batch
					}(),
					3: doneBatch("batch3", "master-session-spawn"),
				},
			},
			audit: func(*runFixture) shuttleengine.ForkAudit {
				return shuttleengine.ForkAudit{NamedSpawns: 1, Forks: forkReports(3)}
			},
			batchesDone: 3,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if result.Outcome != "done" {
					t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
				}
				if warningsContain(result.Warnings, "named-spawn") {
					t.Errorf("Warnings = %v; want no second named-spawn warning", result.Warnings)
				}
				if st := loadRunState(t, fx); len(st.AuditWarnings) != 0 {
					t.Errorf("run-level AuditWarnings = %v; want none", st.AuditWarnings)
				}
				summary := readSummary(t, fx)
				if n := strings.Count(summary, "## Audit warnings"); n != 1 {
					t.Errorf("summary.md carries %d Audit warnings section(s); want 1:\n%s", n, summary)
				}
				if n := strings.Count(summary, "master spawned a named agent"); n != 1 {
					t.Errorf("summary.md carries the batch 2 warning %d times; want once:\n%s", n, summary)
				}
			},
		},
		{
			// A policy finding in the fixer fork's transcript leaves the run done.
			name:    "a nested agent in the fixer fork warns and the run stays done",
			cards:   1,
			session: "master-session-nested",
			prepare: func(t *testing.T, fx *runFixture) { appendIntegrationVerify(t, fx.PlanDir, "true") },
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-nested", CardSHAs: []string{"deadbeef"}},
			}},
			audit: func(*runFixture) shuttleengine.ForkAudit {
				forks := forkReports(1)
				forks = append(forks, shuttleengine.ForkReport{TranscriptPath: "/transcripts/fixer.jsonl", ReportReturned: true, AgentCalls: 1})
				return shuttleengine.ForkAudit{Forks: forks}
			},
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if result.Outcome != "done" {
					t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
				}
				if !warningsContain(result.Warnings, "audit warning (nested-agent)") {
					t.Errorf("Warnings = %v; want the nested-agent warning", result.Warnings)
				}
				if st := loadRunState(t, fx); len(st.AuditWarnings) != 1 {
					t.Fatalf("run-level AuditWarnings = %v; want exactly one", st.AuditWarnings)
				}
				summary := readSummary(t, fx)
				if !strings.Contains(summary, "## Audit warnings") || !strings.Contains(summary, "nested-agent") {
					t.Errorf("summary.md = %q; want an Audit warnings section naming the nested-agent finding", summary)
				}
			},
		},
		{
			// state.json is under _lyx, which nothing the run recorded can check, so the way
			// forward is the --fresh route and never a git restore.
			name:    "a fork writing state.json leaves a pending fork-state-write finding",
			cards:   1,
			session: "master-session-statewrite",
			prepare: func(t *testing.T, fx *runFixture) { appendIntegrationVerify(t, fx.PlanDir, "true") },
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-statewrite", CardSHAs: []string{"deadbeef"}, ForkTranscripts: []string{"/transcripts/fork1.jsonl"}},
			}},
			audit: func(fx *runFixture) shuttleengine.ForkAudit {
				forks := forkReports(2)
				forks[0].WritePaths = []string{filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")}
				return shuttleengine.ForkAudit{Forks: forks}
			},
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				st := loadRunState(t, fx)
				if len(st.PendingAuditFindings) != 1 || st.PendingAuditFindings[0].Class != "fork-state-write" {
					t.Errorf("PendingAuditFindings = %+v; want one fork-state-write finding", st.PendingAuditFindings)
				}
				_, way, _ := strings.Cut(result.StuckReason, "way forward:")
				if !strings.Contains(way, "lyx webster run --fresh") || strings.Contains(way, "accept-audit") {
					t.Errorf("StuckReason = %q; want the --fresh route without accept-audit", result.StuckReason)
				}
			},
		},
		{
			name:    "a fork writing only its own batch's report leaves no pending finding",
			cards:   1,
			session: "master-session-statewrite",
			prepare: func(t *testing.T, fx *runFixture) { appendIntegrationVerify(t, fx.PlanDir, "true") },
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-statewrite", CardSHAs: []string{"deadbeef"}, ForkTranscripts: []string{"/transcripts/fork1.jsonl"}},
			}},
			audit: func(fx *runFixture) shuttleengine.ForkAudit {
				forks := forkReports(2)
				forks[0].WritePaths = []string{filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "batch1"))}
				return shuttleengine.ForkAudit{Forks: forks}
			},
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if st := loadRunState(t, fx); len(st.PendingAuditFindings) != 0 {
					t.Errorf("PendingAuditFindings = %+v; want none", st.PendingAuditFindings)
				}
			},
		},
		{
			// The run-exit fork audit covers the verify-gate fixer fork: a fork outside every batch
			// bracket that writes the plan directory leaves one pending fork-plan-write finding.
			name:    "the fixer fork writing the plan directory is flagged",
			cards:   1,
			session: "master-session-fixer",
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-fixer", CardSHAs: []string{"deadbeef"}, ForkTranscripts: []string{"/transcripts/fork1.jsonl"}},
			}},
			audit: func(fx *runFixture) shuttleengine.ForkAudit {
				forks := forkReports(2)
				forks[1].TranscriptPath = "/transcripts/fixer.jsonl"
				forks[1].WritePaths = []string{filepath.Join(fx.Deps.Geom.PlanDir, "00-overview.md")}
				return shuttleengine.ForkAudit{Forks: forks}
			},
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if result.Outcome != "stuck" {
					t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
				}
				st := loadRunState(t, fx)
				if len(st.PendingAuditFindings) != 1 || st.PendingAuditFindings[0].Class != "fork-plan-write" {
					t.Errorf("PendingAuditFindings = %+v; want one fork-plan-write finding", st.PendingAuditFindings)
				}
			},
		},
		{
			// A fabric reference in the fixer fork's transcript is correctness whatever its command.
			name:  "a fabric reference in the fixer fork is stuck",
			cards: 1,
			prepare: func(t *testing.T, fx *runFixture) {
				fx.Deps.RefMatcher = fabricMatcher{}
				appendIntegrationVerify(t, fx.PlanDir, "true")
			},
			session: "master-session-fabric",
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-fabric", CardSHAs: []string{"deadbeef"}},
			}},
			audit: func(fx *runFixture) shuttleengine.ForkAudit {
				forks := forkReports(2)
				forks[1].TranscriptPath = "/transcripts/fixer.jsonl"
				forks[1].BashCommands = []string{fabricCommand}
				return shuttleengine.ForkAudit{Forks: forks}
			},
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if result.Outcome != "stuck" {
					t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
				}
				if !strings.Contains(result.StuckReason, fabricCommand) {
					t.Errorf("StuckReason = %q; want it to quote %q", result.StuckReason, fabricCommand)
				}
				st := loadRunState(t, fx)
				if len(st.PendingAuditFindings) != 1 || len(st.PendingAuditFindings[0].Paths) != 0 {
					t.Errorf("PendingAuditFindings = %+v; want one finding with no path", st.PendingAuditFindings)
				}
			},
		},
		{
			name:    "the fixer prompt names the gate report and no integration prompt is rendered",
			cards:   1,
			session: "master-session-fixprompt",
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-fixprompt", CardSHAs: []string{"deadbeef"}},
			}},
			audit:       func(*runFixture) shuttleengine.ForkAudit { return shuttleengine.ForkAudit{Forks: forkReports(1)} },
			batchesDone: 1,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				prompt, err := os.ReadFile(filepath.Join(fx.Deps.Geom.PromptsDir, "verify-fix.md"))
				if err != nil {
					t.Fatalf("read verify-fix prompt: %v", err)
				}
				if !strings.Contains(string(prompt), websterengine.VerifyGateReportPath(fx.Deps.Geom.ReportsDir)) {
					t.Errorf("verify-fix prompt does not name the gate report path")
				}
				if _, err := os.Stat(filepath.Join(fx.Deps.Geom.PromptsDir, "integration.md")); err == nil {
					t.Errorf("integration.md exists; Run renders no integration prompt")
				}
			},
		},
		{
			name:    "an acyclic plan reports no cycles and no sequencing warning",
			cards:   2,
			session: "master-session-acyclic",
			state: &websterengine.State{Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
				2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
			}},
			audit:       func(*runFixture) shuttleengine.ForkAudit { return shuttleengine.ForkAudit{Forks: forkReports(2)} },
			batchesDone: 2,
			check: func(t *testing.T, fx *runFixture, result websterengine.RunResult) {
				if len(result.Cycles) != 0 {
					t.Errorf("RunResult.Cycles = %v; want empty for an acyclic plan", result.Cycles)
				}
				if warningsContain(result.Warnings, "dependency cycle") {
					t.Errorf("RunResult.Warnings = %v; want no sequencing-cycle warning for an acyclic plan", result.Warnings)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, tc.cards)
			if tc.prepare != nil {
				tc.prepare(t, fx)
			}
			seedMatchingState(t, fx, tc.state)
			fx.Starter.handle = auditDoneHandle(t, fx, tc.session, tc.batchesDone, tc.audit(fx), func() {})
			seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", tc.session)

			result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			if err != nil {
				t.Fatalf("Run() error = %v; want nil (a correctness finding demotes, it is not an error)", err)
			}
			tc.check(t, fx, result)
		})
	}
}

// expiredShellRun drives fx's Run to a shuttle-done end whose Master result lists labels as expired shells, with Master's last action writing outcomeYAML.
func expiredShellRun(t *testing.T, fx *runFixture, labels []string, outcomeYAML string) websterengine.RunResult {
	t.Helper()
	fx.Deps.ShuttleCfg.BackgroundShellWaitMin = 15
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-shells",
		result: shuttleengine.Result{
			Outcome:       shuttleengine.OutcomeDone,
			SessionID:     "master-session-shells",
			RunDir:        "/run/dir/shells",
			ForkAudit:     &shuttleengine.ForkAudit{Forks: forkReports(1)},
			ExpiredShells: labels,
		},
		onWait: func() {
			writeContractFiles(t, fx, outcomeYAML, "# Shipped\n\nAll good.\n")
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-shells", "master-session-shells")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	return result
}

// The parts of the sentence the friction note and the warning share for the shell `sleep 9999` under a 15-minute bound.
const (
	shellSentenceHead    = "background shell `sleep 9999` ran past `background_shell_wait_min` (15 minutes): the wait stopped waiting and counted Master's turn end, and lyx did not stop the shell; "
	shellStrandRemoved   = "shuttle removes Master's strand when the run finishes, which ends the session and the shell with it; "
	shellStrandReclaimed = "Master's strand stays alive until the next `lyx webster run` reclaims it at entry, which ends the session and the shell with it; "
)

// TestRun_ExpiredShells asserts a waited-out shell states the run's outcome in its friction note and, on every outcome that returns a RunResult, in its warning; that only a done outcome lands in summary.md; and that an empty list adds no warning, summary section or friction note.
func TestRun_ExpiredShells(t *testing.T) {
	t.Parallel()

	t.Run("a waited-out shell warns, lands in the summary and is named in a friction note", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1)
		fx.Deps.FrictionDir = t.TempDir()

		result := expiredShellRun(t, fx, []string{"sleep 9999"}, "outcome: done\nstuck_reason: null\nbatches_done: 1\n")

		want := shellSentenceHead + shellStrandRemoved + "the run's outcome after that turn end: done"
		if !slices.Contains(result.Warnings, want) {
			t.Errorf("Warnings = %v; want %q", result.Warnings, want)
		}
		summary := readSummary(t, fx)
		if !strings.Contains(summary, "## Background shells waited out") || !strings.Contains(summary, "- `sleep 9999`") {
			t.Errorf("summary.md = %q; want the waited-out section naming the shell", summary)
		}
		note, err := os.ReadFile(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md"))
		if err != nil {
			t.Fatalf("read friction note: %v", err)
		}
		if !strings.Contains(string(note), "- "+want+"\n") {
			t.Errorf("friction note = %q; want the same sentence as the warning", note)
		}
	})

	t.Run("Master's own stuck states the outcome with its reason in the note and the warning", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1)
		fx.Deps.FrictionDir = t.TempDir()

		result := expiredShellRun(t, fx, []string{"sleep 9999"}, "outcome: stuck\nstuck_reason: \"batch 1 red\"\nbatches_done: 0\n")

		want := shellSentenceHead + shellStrandRemoved + "the run's outcome after that turn end: stuck (batch 1 red)"
		if !slices.Contains(result.Warnings, want) {
			t.Errorf("Warnings = %v; want %q", result.Warnings, want)
		}
		note, err := os.ReadFile(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md"))
		if err != nil {
			t.Fatalf("read friction note: %v", err)
		}
		if !strings.Contains(string(note), "- "+want+"\n") {
			t.Errorf("friction note = %q; want the same sentence as the warning", note)
		}
		if summary := readSummary(t, fx); strings.Contains(summary, "Background shells waited out") {
			t.Errorf("summary.md = %q; want no waited-out section on a stuck outcome", summary)
		}
	})

	t.Run("a died Master states the error outcome and the reclaim in the note", func(t *testing.T) {
		t.Parallel()
		const (
			strand  = "master-strand-shells-died"
			session = "master-session-shells-died"
		)
		fx := newRunFixture(t, 1)
		fx.Deps.FrictionDir = t.TempDir()
		fx.Deps.ShuttleCfg.BackgroundShellWaitMin = 15
		seedMatchingState(t, fx, &websterengine.State{
			Batches: map[int]*websterengine.BatchState{1: doneBatch("batch1", session)},
		})
		fx.Starter.handle = &runFakeHandle{
			strandGUID: strand,
			result:     shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: session, RunDir: "/run/dir/shells-died", ExpiredShells: []string{"sleep 9999"}},
		}
		seedShuttleRunState(t, fx.ShuttleRunRoot, strand, session)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if !errors.Is(err, websterengine.ErrMasterDied) {
			t.Fatalf("Run() error = %v; want ErrMasterDied", err)
		}

		note, readErr := os.ReadFile(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md"))
		if readErr != nil {
			t.Fatalf("read friction note: %v", readErr)
		}
		want := "- " + shellSentenceHead + shellStrandReclaimed + "the run's outcome after that turn end: error (" + err.Error() + ")\n"
		if !strings.Contains(string(note), want) {
			t.Errorf("friction note = %q; want %q", note, want)
		}
	})

	t.Run("no expired shell writes nothing", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1)
		fx.Deps.FrictionDir = t.TempDir()

		result := expiredShellRun(t, fx, nil, "outcome: done\nstuck_reason: null\nbatches_done: 1\n")

		if warningsContain(result.Warnings, "background_shell_wait_min") {
			t.Errorf("Warnings = %v; want no expired-shell warning", result.Warnings)
		}
		if summary := readSummary(t, fx); strings.Contains(summary, "Background shells waited out") {
			t.Errorf("summary.md = %q; want no waited-out section", summary)
		}
		if _, err := os.Stat(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md")); !os.IsNotExist(err) {
			t.Errorf("friction note stat error = %v; want not-exist", err)
		}
	})
}

// TestRun_PausedOutcomeLeavesPauseFlagIntact proves a genuinely mid-run pause request (one
// requested WHILE Master is working, i.e.
// present again by the time Master's own "outcome: paused" final action lands — Run's own pre-spawn
// commitment-point clear already ran before Master ever started) is left intact by the post-run
// mapping: the operator's own record that a pause is still pending, never silently cleared out from
// under them.
func TestRun_PausedOutcomeLeavesPauseFlagIntact(t *testing.T) {
	t.Parallel()
	fx := newRunFixture(t, 1)

	handle := &runFakeHandle{
		strandGUID: "master-strand-paused",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-paused",
			RunDir:    "/run/dir/paused",
		},
		onWait: func() {
			// Simulate an operator pausing DURING Master's own run: the
			// pre-spawn ClearPause already ran before Master started, so
			// this is a genuinely new request Master's own paused final
			// action is responding to.
			if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
				t.Fatalf("RequestPause() error = %v", err)
			}
			writeContractFiles(t, fx, "outcome: paused\nstuck_reason: null\nbatches_done: 0\n", "# Paused mid-run\n")
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-paused", "master-session-paused")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "paused" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "paused")
	}
	if !websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
		t.Error("pause flag cleared on a paused outcome; want it left intact as the operator's own record")
	}
}

// auditDoneHandle builds a done Master handle whose onWait writes outcome.yaml (batchesDone batches) and a valid summary.md, with audit as the whole-session fork audit.
func auditDoneHandle(t *testing.T, fx *runFixture, session string, batchesDone int, audit shuttleengine.ForkAudit, extra func()) *runFakeHandle {
	t.Helper()
	return &runFakeHandle{
		strandGUID: "master-strand-audit",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: session,
			RunDir:    "/run/dir/audit",
			ForkAudit: &audit,
		},
		onWait: func() {
			if extra != nil {
				extra()
			}
			writeContractFiles(t, fx, fmt.Sprintf("outcome: done\nstuck_reason: null\nbatches_done: %d\n", batchesDone), "# Shipped\n\nAll good.\n")
		},
	}
}

// TestRun_DoneWithParentWriteToTrackedFileDemotesToStuck proves the run-exit audit demotes a done outcome to stuck on an undispositioned correctness finding — a Master write into a tracked file — naming the path and the git way forward,
// that the finding stays pending and refuses a bare re-run, and that once the file is restored with git and the finding accepted a re-run with a clean audit ends done.
func TestRun_DoneWithParentWriteToTrackedFileDemotesToStuck(t *testing.T) {
	t.Parallel()
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-violation",
				Digest: &websterengine.Digest{Status: "done", HeadSHA: fx.Git.head}},
		},
	})
	tracked := filepath.Join(fx.Worktree, "base.txt")
	forks := forkReports(1)

	fx.Starter.handle = auditDoneHandle(t, fx, "master-session-violation", 1,
		shuttleengine.ForkAudit{ParentWrites: []string{tracked}, Forks: forks},
		func() {
			if err := os.WriteFile(tracked, []byte("hand-edited by master"), 0o644); err != nil {
				t.Fatalf("edit tracked file: %v", err)
			}
			fx.Git.differing[tracked] = true
		})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", "master-session-violation")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a correctness finding demotes, it is not an error)", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
	}
	if !strings.Contains(result.StuckReason, tracked) || !strings.Contains(result.StuckReason, "git") {
		t.Errorf("StuckReason = %q; want it to name %s and the git way forward", result.StuckReason, tracked)
	}

	if !strings.Contains(result.StuckReason, "lyx webster accept-audit") {
		t.Errorf("StuckReason = %q; want it to name lyx webster accept-audit", result.StuckReason)
	}
	st := loadRunState(t, fx)
	if len(st.PendingAuditFindings) != 1 || len(st.PendingAuditFindings[0].Paths) != 1 || st.PendingAuditFindings[0].Paths[0] != tracked {
		t.Fatalf("PendingAuditFindings = %+v; want one finding naming %s", st.PendingAuditFindings, tracked)
	}

	// A bare re-step without accepting is refused and spawns no Master.
	before := fx.Starter.callCount()
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("second Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), tracked) || !strings.Contains(err.Error(), "lyx webster accept-audit") {
		t.Errorf("second Run() error = %q; want it to name %s and lyx webster accept-audit", err, tracked)
	}
	if got := fx.Starter.callCount(); got != before {
		t.Errorf("Starter calls = %d after the refused Run; want %d", got, before)
	}

	if _, _, err := websterengine.AcceptPendingAudit(nil, st, fx.Deps.Geom, nil); !errors.Is(err, websterengine.ErrAuditNotAcceptable) || !strings.Contains(err.Error(), tracked) {
		t.Fatalf("AcceptPendingAudit() before the revert error = %v; want ErrAuditNotAcceptable naming %s", err, tracked)
	}
	writeWorktreeFile(t, fx.Worktree, "base.txt", "base")
	fx.Git.differing[tracked] = false
	if _, _, err := websterengine.AcceptPendingAudit(nil, st, fx.Deps.Geom, nil); err != nil {
		t.Fatalf("AcceptPendingAudit() after the revert error = %v; want nil", err)
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Starter.handle = auditDoneHandle(t, fx, "master-session-violation", 1, shuttleengine.ForkAudit{Forks: forks}, nil)
	result, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("third Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("third RunResult.Outcome = %q; want %q after the file was restored and the finding accepted", result.Outcome, "done")
	}
	st = loadRunState(t, fx)
	if len(st.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %+v; want none", st.PendingAuditFindings)
	}
}

// writeDoneContract writes the two files Merriam's last action writes, outcome done.
func writeDoneContract(t *testing.T, fx *runFixture) {
	t.Helper()
	writeContractFiles(t, fx, "outcome: done\nstuck_reason: null\nbatches_done: 1\n", "# Shipped\n\nAll good.\n")
}

// runToDone drives fx's Run to a done outcome with the given ForkAudit forks, scripting
// outcome.yaml/summary.md the same way auditDoneHandle does, and returns the RunResult.
func runToDone(t *testing.T, fx *runFixture, strandGUID, sessionID string, forks []shuttleengine.ForkReport, batchesDone int) websterengine.RunResult {
	t.Helper()

	handle := &runFakeHandle{
		strandGUID: strandGUID,
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: sessionID,
			RunDir:    "/run/dir/" + sessionID,
			ForkAudit: &shuttleengine.ForkAudit{Forks: forks},
		},
		onWait: func() {
			writeContractFiles(t, fx, fmt.Sprintf("outcome: done\nstuck_reason: null\nbatches_done: %d\n", batchesDone), "# Shipped\n\nAll good.\n")
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, strandGUID, sessionID)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	return result
}

// requireWayForward fails unless err carries the trailing "way forward:" clause and every want fragment after it, so each reaching test matches the message the way the refusal table does.
func requireWayForward(t *testing.T, err error, wants ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil; want a refusal carrying a way forward")
	}
	msg := err.Error()
	_, clause, found := strings.Cut(msg, "way forward:")
	if !found {
		t.Fatalf("error = %q; want a trailing way forward clause", msg)
	}
	for _, want := range wants {
		if !strings.Contains(clause, want) {
			t.Errorf("way forward clause = %q; want it to contain %q", clause, want)
		}
	}
}

// askingMaster scripts fx's Starter with a Master that ends its turn asking, the cheapest way for a re-run to prove it got past every refusal gate and reached the spawn.
func askingMaster(t *testing.T, fx *runFixture, label string) {
	t.Helper()
	fx.Starter.startErr = nil
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-" + label,
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeAsking,
			SessionID: "master-session-" + label,
			RunDir:    "/run/dir/" + label,
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-"+label, "master-session-"+label)
}

// requireReachedMaster asserts err is the Master-asking error of a run that got past every gate.
func requireReachedMaster(t *testing.T, fx *runFixture, err error) {
	t.Helper()
	if !errors.Is(err, websterengine.ErrMasterAsking) {
		t.Fatalf("Run() after taking the way forward error = %v; want it to reach the Master spawn", err)
	}
	if fx.Starter.callCount() == 0 {
		t.Error("Starter was never reached after taking the way forward")
	}
}

// rebaselineOnDisk is what the rebaseline verb does: parse the edited plan, re-derive its batches, restamp the recorded fingerprint and save.
func rebaselineOnDisk(t *testing.T, fx *runFixture, cards ...int) {
	t.Helper()
	plan, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	st := loadRunState(t, fx)
	batches, _ := websterengine.SequenceBatches(fx.Deps.Batcher.Batch(plan.Cards))
	if _, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: plan, Batches: batches, State: st, Cards: cards, Geom: fx.Deps.Geom}); err != nil {
		t.Fatalf("Rebaseline() error = %v", err)
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
}

// emptyBatcher is a batchifier that derives no execution batches from any plan.
type emptyBatcher struct{}

func (emptyBatcher) Batch([]planparser.Card) []batcher.Batch { return nil }
func (emptyBatcher) Name() string                            { return "empty" }

// TestRun_RunExitRefusals reaches each run-exit refusal over a done Master, asserts its message and way forward, then takes the way forward (the state a re-driven batch leaves, a finished summary, an audit that completed) and proves the re-run ends done; a resume whose prior session's batches fall outside the audit passes outright.
func TestRun_RunExitRefusals(t *testing.T) {
	t.Parallel()

	const session = "master-session-exit"
	const strand = "master-strand-exit"
	const doneOutcome = "outcome: done\nstuck_reason: null\nbatches_done: 1\n"
	doneRecord := func() *websterengine.BatchState { return doneBatch("batch1", session) }

	tests := []struct {
		name  string
		cards int
		state map[int]*websterengine.BatchState
		audit *shuttleengine.ForkAudit
		// outcome is outcome.yaml's content; "" writes none.
		outcome string
		summary bool
		// wantOK expects Run to succeed outright, with no refusal to take a way forward from.
		wantOK      bool
		msgContains []string
		want        string
		// repair mutates the world the way the way forward describes, before the clean re-run.
		repair func(t *testing.T, fx *runFixture)
		// rerunBatches is how many batches and forks the clean re-run covers; zero means one.
		rerunBatches int
	}{
		{
			name:    "malformed outcome",
			cards:   1,
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   &shuttleengine.ForkAudit{Forks: forkReports(1)},
			outcome: "outcome: [not a mapping\n",
			summary: true,
			want:    "stale file is archived",
		},
		{
			name:        "missing summary",
			cards:       1,
			state:       map[int]*websterengine.BatchState{1: doneRecord()},
			audit:       &shuttleengine.ForkAudit{Forks: forkReports(1)},
			outcome:     doneOutcome,
			summary:     false,
			msgContains: []string{"summary"},
			want:        "re-drives every batch without a done record",
		},
		{
			// A Master that writes outcome: done while a plan batch has no terminal done record
			// (begun-but-never-recorded — a fork that slipped past record-batch) is refused even
			// when the outcome/summary files are well-formed and the audit is clean.
			name:        "batch without a done record",
			cards:       1,
			state:       map[int]*websterengine.BatchState{1: {Slug: "batch1", Kind: "fork", SessionID: session}},
			audit:       &shuttleengine.ForkAudit{Forks: forkReports(1)},
			outcome:     doneOutcome,
			summary:     true,
			msgContains: []string{"terminal done record"},
			repair: func(t *testing.T, fx *runFixture) {
				st := loadRunState(t, fx)
				st.Batches[1] = doneRecord()
				if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
			},
			want: "re-drives every batch without a done record",
		},
		{
			name:    "audit never completed",
			cards:   1,
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   nil,
			outcome: doneOutcome,
			summary: true,
			want:    "re-drives every batch without a done record",
		},
		{
			name:        "audited fewer forks than begun",
			cards:       1,
			state:       map[int]*websterengine.BatchState{1: doneRecord()},
			audit:       &shuttleengine.ForkAudit{},
			outcome:     doneOutcome,
			summary:     true,
			msgContains: []string{"fewer than"},
			want:        "re-drives every batch without a done record",
		},
		{
			// The audit covers only the fresh session's own subagents, so a crash-resumed run's
			// batches the prior session completed must not count against it.
			name:  "a resume excludes the prior session's batches from the audit",
			cards: 2,
			state: map[int]*websterengine.BatchState{
				1: doneBatch("batch1", "master-session-crashed"),
				2: doneBatch("batch2", session),
			},
			audit:   &shuttleengine.ForkAudit{Forks: forkReports(1)},
			outcome: "outcome: done\nstuck_reason: null\nbatches_done: 2\n",
			summary: true,
			wantOK:  true,
		},
		{
			name:  "the current session's own shortfall still fails",
			cards: 2,
			state: map[int]*websterengine.BatchState{
				1: doneBatch("batch1", session),
				2: doneBatch("batch2", session),
			},
			audit:        &shuttleengine.ForkAudit{Forks: forkReports(1)},
			outcome:      "outcome: done\nstuck_reason: null\nbatches_done: 2\n",
			summary:      true,
			msgContains:  []string{"fewer than"},
			want:         "re-drives every batch without a done record",
			rerunBatches: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, tt.cards)
			seedMatchingState(t, fx, &websterengine.State{Batches: tt.state})
			fx.Starter.handle = &runFakeHandle{
				strandGUID: strand,
				result: shuttleengine.Result{
					Outcome:   shuttleengine.OutcomeDone,
					SessionID: session,
					RunDir:    "/run/dir/exit",
					ForkAudit: tt.audit,
				},
				onWait: func() {
					if tt.outcome != "" {
						if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(tt.outcome), 0o644); err != nil {
							t.Fatalf("write outcome.yaml: %v", err)
						}
					}
					if tt.summary {
						if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped\n\nAll good.\n"), 0o644); err != nil {
							t.Fatalf("write summary.md: %v", err)
						}
					}
				},
			}
			seedShuttleRunState(t, fx.ShuttleRunRoot, strand, session)

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Run() = %v; want nil", err)
				}
				return
			}
			requireWayForward(t, err, "lyx webster run", tt.want)
			for _, want := range tt.msgContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Run() error = %q; want it to contain %q", err, want)
				}
			}

			if tt.repair != nil {
				tt.repair(t, fx)
			}
			rerunBatches := max(tt.rerunBatches, 1)
			runToDone(t, fx, strand, session, forkReports(rerunBatches), rerunBatches)
		})
	}
}

// TestRun_FreshRunOverNewGenerationAfterArchive proves a rework generation gets a fresh run:
// a finished two-card run is archived with ArchiveRunRecord, the plan is replaced by a generation whose first_card is 3,
// and Run starts over that plan with no ErrFingerprintMismatch, recording and telling Master only the new generation's batches.
func TestRun_FreshRunOverNewGenerationAfterArchive(t *testing.T) {
	t.Parallel()
	fx := newRunFixture(t, 2)
	appendIntegrationVerify(t, fx.PlanDir, "go test ./...")
	geom := fx.Deps.Geom

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})
	outcomePath := filepath.Join(geom.WebsterDir, "outcome.yaml")
	if err := os.WriteFile(outcomePath, []byte("outcome: done\nstuck_reason: null\nbatches_done: 2\n"), 0o644); err != nil {
		t.Fatalf("seed outcome: %v", err)
	}

	archiveDir := filepath.Join(t.TempDir(), "prior-generation", "webster")
	if err := websterengine.ArchiveRunRecord(geom, archiveDir); err != nil {
		t.Fatalf("ArchiveRunRecord() error = %v", err)
	}

	// Replace the plan with a whole new generation numbered on from the retired one.
	entries, err := os.ReadDir(fx.PlanDir)
	if err != nil {
		t.Fatalf("read plan dir: %v", err)
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(fx.PlanDir, e.Name())); err != nil {
			t.Fatalf("remove %s: %v", e.Name(), err)
		}
	}
	plankit.Write(t, fx.PlanDir, runPlan(3, 2))
	appendIntegrationVerify(t, fx.PlanDir, "go test ./...")

	fx.Starter.handle = &runFakeHandle{strandGUID: "master-strand-generation", waitErr: fmt.Errorf("stop after spawn")}
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("Run() error = %v; want no ErrFingerprintMismatch over a fresh generation", err)
	}
	if fx.Starter.callCount() != 1 {
		t.Fatalf("Starter.callCount() = %d; want Master spawned once (Run error: %v)", fx.Starter.callCount(), err)
	}

	st := loadRunState(t, fx)
	for number := range st.Batches {
		if number != 3 && number != 4 {
			t.Errorf("state records batch %d; want only the new generation's batches 3 and 4", number)
		}
	}

	prompt := fx.Starter.startCalls[0].Prompt
	for _, want := range []string{"03 — batch3", "04 — batch4"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Master prompt lacks %q", want)
		}
	}
	for _, stale := range []string{"batch1", "batch2"} {
		if strings.Contains(prompt, stale) {
			t.Errorf("Master prompt names retired batch %q; want only the new generation's", stale)
		}
	}
}

// seedFreshPendingState seeds a state with one recorded batch started at the fixture's first commit and one pending finding naming paths, and returns that start commit.
func seedFreshPendingState(t *testing.T, fx *runFixture, paths ...string) string {
	t.Helper()
	start := fx.Git.head
	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "stale-run",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", StartSHA: start},
		},
		PendingAuditFindings: []websterengine.PendingAuditFinding{{ID: "sess/parent:write:1", Class: "parent-write", Detail: "master wrote a tracked file", Paths: paths}},
	})
	return start
}

// seedUncheckableState seeds a state with one batch failed on an uncheckable finding, started at the fixture's first commit, and returns that start commit.
func seedUncheckableState(t *testing.T, fx *runFixture) string {
	t.Helper()
	start := fx.Git.head
	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "stale-run",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug: "batch1", Kind: "fork", StartSHA: start, Terminal: true, Status: websterengine.DigestStatusFailed,
				Digest:      &websterengine.Digest{Batch: "01-batch1", Status: websterengine.DigestStatusFailed, HeadSHA: start},
				Uncheckable: []string{"fabric-reference: cat FABRICREF/webster/state.json"},
			},
		},
	})
	return start
}

// requireFreshRefusal asserts a --fresh run was refused with ErrPendingAuditFindings and archived nothing: state.json is still in place, and so is marker when one was planted in the reports dir.
func requireFreshRefusal(t *testing.T, fx *runFixture, err error, marker string) {
	t.Helper()
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}
	if marker != "" {
		if _, statErr := os.Stat(marker); statErr != nil {
			t.Errorf("reports dir was archived: %v", statErr)
		}
	}
	if got := fx.Starter.callCount(); got != 0 {
		t.Errorf("Starter calls = %d; want 0", got)
	}
}

// requireReinitialisedRun asserts the state records no pending finding and is no longer the stale run seeded by seedFreshPendingState or seedUncheckableState.
func requireReinitialisedRun(t *testing.T, fx *runFixture) {
	t.Helper()
	st := loadRunState(t, fx)
	if len(st.PendingAuditFindings) != 0 || st.RunGUID == "stale-run" {
		t.Errorf("state = %+v; want a re-initialised run with no pending finding", st)
	}
}

// plantReportsMarker writes a file into the reports dir, so a test can tell whether the dir was archived.
func plantReportsMarker(t *testing.T, fx *runFixture) string {
	t.Helper()
	marker := filepath.Join(fx.Deps.Geom.ReportsDir, "marker.yaml")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return marker
}

// TestRun_FreshOverPendingFindings asserts how Run weighs a pending audit finding or a batch failed on an uncheckable finding against --fresh and a plain re-run:
// --fresh refuses, archiving nothing, while the evidence still stands, names the way forward, and drops the finding once the evidence is cleared.
// Subtests share the logger's process-global output, so none runs in parallel.
func TestRun_FreshOverPendingFindings(t *testing.T) {
	t.Run("refuses while the suspect path differs from the start commit", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		tracked := filepath.Join(fx.Worktree, "base.txt")
		start := seedFreshPendingState(t, fx, tracked)
		fx.Git.commit()
		fx.Git.differing[tracked] = true
		marker := plantReportsMarker(t, fx)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, marker)
		if !strings.Contains(err.Error(), tracked) || !strings.Contains(err.Error(), start) {
			t.Errorf("Run() error = %q; want it to name %s and the start commit %s", err, tracked, start)
		}
	})

	t.Run("refuses a commit past the start even with the suspect file restored", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		tracked := filepath.Join(fx.Worktree, "base.txt")
		start := seedFreshPendingState(t, fx, tracked)
		fx.Git.commit()
		marker := plantReportsMarker(t, fx)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, marker)
		if !strings.Contains(err.Error(), "is not the run's start commit "+start) {
			t.Errorf("Run() error = %q; want it to name the start commit %s", err, start)
		}
		requireWayForward(t, err, "1) lyx webster reset --to start", "2) lyx webster run --fresh")
	})

	t.Run("drops the finding once the branch is reset to the start commit", func(t *testing.T) {
		const session = "master-session-fresh"
		fx := newRunFixture(t, 1)
		tracked := filepath.Join(fx.Worktree, "base.txt")
		start := seedFreshPendingState(t, fx, tracked)
		fx.Git.commit()
		fx.Git.head = start

		fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forkReports(1)}, func() {
			st := loadRunState(t, fx)
			if len(st.PendingAuditFindings) != 0 {
				t.Errorf("PendingAuditFindings = %+v; want none in the fresh state", st.PendingAuditFindings)
			}
			st.Batches[1] = doneBatch("batch1", session)
			if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}
		})
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

		result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if fx.Starter.callCount() == 0 {
			t.Error("Starter was never reached")
		}
		if !warningsContain(result.Warnings, "--fresh dropped pending audit finding sess/parent:write:1") {
			t.Errorf("Warnings = %v; want the dropped finding named", result.Warnings)
		}
		requireReinitialisedRun(t, fx)
	})

	t.Run("drops a pathless finding on an unchanged plan", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		seedFreshPendingState(t, fx)
		askingMaster(t, fx, "pathless")
		logs := captureLogs(t)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		// Master ends asking, so no RunResult carries the drop warning; the log does.
		if !strings.Contains(logs.String(), "--fresh dropped pending audit finding sess/parent:write:1") {
			t.Errorf("log = %q; want the drop warning naming the finding", logs.String())
		}
		requireReinitialisedRun(t, fx)
	})

	t.Run("refuses a contract file no Master write cleared, then drops it once absent", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		contract := websterengine.OutcomePath(fx.Deps.Geom.WebsterDir)
		seedFreshPendingState(t, fx, contract)
		if err := os.WriteFile(contract, []byte("outcome: done\n"), 0o644); err != nil {
			t.Fatalf("write contract file: %v", err)
		}

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, "")
		requireWayForward(t, err, "rm "+contract, "lyx webster run --fresh")

		if err := os.Remove(contract); err != nil {
			t.Fatalf("remove contract file: %v", err)
		}
		askingMaster(t, fx, "contract absent")
		_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
	})

	t.Run("refuses a differing plan path with restore-plan, then drops it once the plan is restored", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		card := filepath.Join(fx.PlanDir, "01-batch1.md")
		seedFreshPendingState(t, fx, card)
		addCardUses(t, fx.PlanDir, 1, "base.txt")

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, "")
		if !strings.Contains(err.Error(), "restore-plan") || strings.Contains(err.Error(), "git checkout") {
			t.Errorf("Run() error = %q; want restore-plan and no git checkout", err)
		}

		if _, err := websterengine.RestorePlan(loadRunState(t, fx), fx.Deps.Geom); err != nil {
			t.Fatalf("RestorePlan() error = %v", err)
		}
		askingMaster(t, fx, "restored")
		_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		requireReinitialisedRun(t, fx)
	})

	t.Run("drops a differing plan path whose recorded copy is missing", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		card := filepath.Join(fx.PlanDir, "01-batch1.md")
		seedFreshPendingState(t, fx, card)
		addCardUses(t, fx.PlanDir, 1, "base.txt")
		if err := os.RemoveAll(filepath.Join(fx.Deps.Geom.WebsterDir, "plan-baseline")); err != nil {
			t.Fatalf("empty the plan baseline store: %v", err)
		}
		askingMaster(t, fx, "no copy")
		logs := captureLogs(t)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		if !strings.Contains(logs.String(), card) || !strings.Contains(logs.String(), "recorded copy is missing") {
			t.Errorf("log = %q; want the drop warning naming %s and the missing copy", logs.String(), card)
		}
		requireReinitialisedRun(t, fx)
	})

	t.Run("a plain re-run over a pending plan path names restore-plan and rebaseline", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		seedFreshPendingState(t, fx, filepath.Join(fx.PlanDir, "01-batch1.md"))

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
			t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
		}
		requireWayForward(t, err, "restore-plan", "rebaseline --card")
		if strings.Contains(err.Error(), "git checkout") || strings.Contains(err.Error(), "with git") {
			t.Errorf("Run() error = %q; want no git way forward for a plan path", err)
		}
	})

	t.Run("drops a batch failed on an uncheckable finding when HEAD is the start commit", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		seedUncheckableState(t, fx)
		askingMaster(t, fx, "uncheckable")
		logs := captureLogs(t)

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		if !strings.Contains(logs.String(), "--fresh dropped batch 01's uncheckable findings: fabric-reference: cat FABRICREF/webster/state.json") {
			t.Errorf("log = %q; want the drop warning naming batch 01 and its entry", logs.String())
		}
		st := loadRunState(t, fx)
		if bs := st.Batches[1]; st.RunGUID == "stale-run" || (bs != nil && len(bs.Uncheckable) != 0) {
			t.Errorf("state = %+v; want a re-initialised run without the failed batch", st)
		}
	})

	t.Run("refuses an uncheckable batch with HEAD past the start commit", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		start := seedUncheckableState(t, fx)
		fx.Git.commit()

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, "")
		if !strings.Contains(err.Error(), "is not the run's start commit "+start) {
			t.Errorf("Run() error = %q; want it to name the start commit %s", err, start)
		}
	})

	t.Run("divergent starts need HEAD before every one", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		root := fx.Git.head
		left := fx.Git.commit()
		fx.Git.head = root
		right := fx.Git.commit()
		seedMatchingState(t, fx, &websterengine.State{
			RunGUID: "stale-run",
			Batches: map[int]*websterengine.BatchState{
				1: {
					Slug: "batch1", Kind: "fork", StartSHA: left, Terminal: true, Status: websterengine.DigestStatusFailed,
					Uncheckable: []string{"fabric-reference: cat FABRICREF/webster/state.json"},
				},
				2: {Slug: "batch2", Kind: "fork", StartSHA: right},
			},
		})

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireFreshRefusal(t, fx, err, "")
		requireWayForward(t, err, "1) lyx webster reset --to start", "2) lyx webster run --fresh")
		if strings.Contains(err.Error(), "merge-base --octopus") {
			t.Errorf("Run() error = %q; want the reset verb, not a git merge-base command", err)
		}

		fx.Git.head = root
		askingMaster(t, fx, "divergent")
		_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		if st := loadRunState(t, fx); st.RunGUID == "stale-run" {
			t.Errorf("RunGUID = %q; want a re-initialised run", st.RunGUID)
		}
	})

	t.Run("--fresh stays a no-op on an unchanged plan with nothing pending", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		seedMatchingState(t, fx, &websterengine.State{RunGUID: "kept-run"})
		askingMaster(t, fx, "resume")

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
		requireReachedMaster(t, fx, err)
		if st := loadRunState(t, fx); st.RunGUID != "kept-run" {
			t.Errorf("RunGUID = %q; want %q kept", st.RunGUID, "kept-run")
		}
	})
}
