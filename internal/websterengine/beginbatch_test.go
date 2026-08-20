//go:build integration

// beginbatch_test.go exercises BeginBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine HeadSHA capture, while the model-injection
// seam (Injector) and the provider seam (shuttleengine.Engine) are local
// fakes, webster's own package-local injection and provider fixture pattern. The plan
// itself is a minimal *planparser.Plan (Dir only — begin-batch never reads
// Plan.Cards, only deps.Batches, the already-derived execution batches),
// backed by a t.TempDir() seeded with a throwaway markdown file so the
// fingerprint gate has something real to hash. There is no chain/restart
// path and no oversized role under the flat card-list model: this file's
// own mustFingerprint helper duplicates fingerprint.go's pure hashing
// algorithm rather than importing anything, since this file deliberately
// stays in the external websterengine_test package (fingerprint itself is
// package-private).

package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newScratchRepo initializes a fresh git repo in a t.TempDir() and
// configures a throwaway committer identity, returning its path — kept
// package-local rather than shared, since test-helper packages are
// deliberately not shared across modules.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// mustGit runs a git command in dir via gitexec.RunGit, failing the test on
// any spawn error or non-zero exit, and returns stdout.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	if exitCode != 0 {
		t.Fatalf("git %v in %s exited %d: %s", args, dir, exitCode, stderr)
	}
	return stdout
}

// commitFile writes name=content into dir and commits it with message,
// returning the resulting commit SHA.
func commitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}

// seedPlanDir creates a t.TempDir() seeded with one throwaway markdown file,
// so the fingerprint gate has something real to hash — BeginBatch's
// fingerprint gate reads planDir directly, never deps.Batches, so no actual
// plan-format parsing is needed for these tests (per the card's own literal-
// Batches-value requirement).
func seedPlanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("seed plan dir: %v", err)
	}
	return dir
}

// mustFingerprint replicates fingerprint.go's own algorithm exactly
// (SHA-256 over every "*.md" file's name and contents, sorted, NUL-
// separated) so a test can seed a State.PlanFingerprint that matches what
// BeginBatch will independently recompute — fingerprint itself is
// package-private and this file's tests deliberately stay in the external
// websterengine_test package (see the file's own doc comment).
func mustFingerprint(t *testing.T, planDir string) string {
	t.Helper()
	entries, err := os.ReadDir(planDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// beginFakeReed is a minimal shuttleengine.ReedOps double for BeginBatch's
// strand-reclaim step: Status returns a scripted set of live strands, and
// RemoveStrand records every guid it was asked to stop. Only Status and
// RemoveStrand are reached by BeginBatch's own path.
type beginFakeReed struct {
	live    []string
	removed []string
}

func (m *beginFakeReed) Status() (reedengine.StatusResult, error) {
	var strands []reedengine.StrandStatus
	for _, g := range m.live {
		strands = append(strands, reedengine.StrandStatus{GUID: g, Live: true})
	}
	return reedengine.StatusResult{Strands: strands}, nil
}
func (m *beginFakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	m.removed = append(m.removed, guid)
	return reedengine.Removed{}, nil
}
func (m *beginFakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return reedengine.Strand{}, nil
}
func (m *beginFakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *beginFakeReed) SendKey(guid, key string) error                { return nil }
func (m *beginFakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*beginFakeReed)(nil)

// beginCard returns a minimal single-card batcher.Batch identifying number
// and slug — begin-batch's batchIdentity assumption (batch ≡ card under the
// identity batchifier).
func beginCard(number int, slug string) batcher.Batch {
	return batcher.Batch{Cards: []planparser.Card{{Number: number, Slug: slug, Title: slug, Intent: "placeholder card " + slug}}}
}

// beginFakeInjector is a hermetic websterengine.Injector double: it records
// every (guid, inputs) call so a test can assert exactly how many times, and
// with what model-switch sequence, BeginBatch injected into Master's pane.
type beginFakeInjector struct {
	calls []injectCall
	err   error
}

type injectCall struct {
	GUID   string
	Inputs []shuttleengine.PaneInput
}

func (f *beginFakeInjector) Inject(guid string, inputs []shuttleengine.PaneInput) error {
	f.calls = append(f.calls, injectCall{GUID: guid, Inputs: inputs})
	return f.err
}

var _ websterengine.Injector = (*beginFakeInjector)(nil)

// beginFakeEngine is a hermetic shuttleengine.Engine double: ModelSwitchSequence
// returns a recognizable marker sequence naming the requested model, so a test
// can assert BeginBatch requested the correct target model without decoding
// real provider grammar; every other method is unreached by BeginBatch's own
// path and returns a fixed, inert value.
type beginFakeEngine struct{}

func (e *beginFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *beginFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) { return nil, nil }
func (e *beginFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *beginFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *beginFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *beginFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}
func (e *beginFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *beginFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// ModelSwitchSequence returns a single marker PaneInput naming model, so a test asserting
// BeginBatch's Injector call can read the target model back out of the recorded inputs.
func (e *beginFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: "/model " + model, Submit: true}}
}

var _ shuttleengine.Engine = (*beginFakeEngine)(nil)

// beginFixture is a fully-wired set of BeginBatch dependencies: a real
// scratch git repo as WorktreeRoot, fresh webster/reports/prompts temp dirs,
// two literal single-card execution batches backed by a seeded plan dir for
// the fingerprint gate, and webster's two roles pre-resolved with distinct
// model names.
type beginFixture struct {
	Deps      websterengine.BeginDeps
	Injector  *beginFakeInjector
	Reed      *beginFakeReed
	Worktree  string
	PlanDir   string
	PromptDir string
}

func newBeginFixture(t *testing.T) *beginFixture {
	t.Helper()

	planDir := seedPlanDir(t)
	fp := mustFingerprint(t, planDir)

	plan := &planparser.Plan{Dir: planDir}
	batches := []batcher.Batch{
		beginCard(1, "json-flag"),
		beginCard(2, "list-tests"),
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{}},
	}

	injector := &beginFakeInjector{}
	promptsDir := t.TempDir()
	reed := &beginFakeReed{}

	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before BeginBatch reaches RenderForkPrompt.
	hubPath := filepath.Dir(worktree)
	seedHubStencils(t, hubPath)

	deps := websterengine.BeginDeps{
		Plan:     plan,
		Batches:  batches,
		State:    &websterengine.State{PlanFingerprint: fp, MasterStrand: "master-strand-1"},
		Roles:    roles,
		Config:   websterengine.Config{SelfFixCap: 2},
		Engine:   &beginFakeEngine{},
		Injector: injector,
		Reed:     reed,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   promptsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			PlanDir:      planDir,
		},
	}

	return &beginFixture{Deps: deps, Injector: injector, Reed: reed, Worktree: worktree, PlanDir: planDir, PromptDir: promptsDir}
}

// TestBeginBatch_PauseSentinel proves the pause gate fires before anything else — including before
// the Injector is ever reached — and that the returned error satisfies errors.Is(err,
// websterengine.ErrPaused).
func TestBeginBatch_PauseSentinel(t *testing.T) {
	fx := newBeginFixture(t)

	if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
		t.Fatalf("RequestPause() error = %v; want nil", err)
	}

	_, err := websterengine.BeginBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrPaused) {
		t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPaused)", err)
	}
	if len(fx.Injector.calls) != 0 {
		t.Errorf("Injector was reached (%d calls) while paused; want zero", len(fx.Injector.calls))
	}
}

// TestBeginBatch_FingerprintMismatch proves a plan edited after run init is refused at begin-batch
// entry with the ErrFingerprintMismatch sentinel, before the Injector is ever reached.
func TestBeginBatch_FingerprintMismatch(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"

	_, err := websterengine.BeginBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if !strings.Contains(err.Error(), "--fresh") {
		t.Errorf("BeginBatch() error = %q; want it to point at run --fresh", err.Error())
	}
	if len(fx.Injector.calls) != 0 {
		t.Errorf("Injector was reached (%d calls) on a fingerprint mismatch; want zero", len(fx.Injector.calls))
	}
}

// TestBeginBatch_ModelAssertion proves the idempotent model-assertion rule: a begin-batch call
// injects exactly once when AssertedModel still names a different model, updating AssertedModel
// afterward, while a repeat call once AssertedModel already names the target model injects zero
// times.
// There is no oversized escalation under the flat card-list model — every batch targets the same
// RoleMaster model.
func TestBeginBatch_ModelAssertion(t *testing.T) {
	t.Run("first call injects and updates AssertedModel", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.State.AssertedModel = "some-other-model"

		result, err := websterengine.BeginBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if len(fx.Injector.calls) != 1 {
			t.Fatalf("Injector.calls = %d; want exactly 1", len(fx.Injector.calls))
		}
		call := fx.Injector.calls[0]
		if call.GUID != "master-strand-1" {
			t.Errorf("Inject guid = %q; want %q", call.GUID, "master-strand-1")
		}
		if len(call.Inputs) != 1 || !strings.Contains(call.Inputs[0].Text, "master-model") {
			t.Errorf("Inject inputs = %v; want the master-model switch sequence", call.Inputs)
		}
		if fx.Deps.State.AssertedModel != "master-model" {
			t.Errorf("State.AssertedModel = %q; want %q", fx.Deps.State.AssertedModel, "master-model")
		}
		if result.AssertedModel != "master-model" {
			t.Errorf("BeginResult.AssertedModel = %q; want %q", result.AssertedModel, "master-model")
		}
	})

	t.Run("same-model batch injects zero times (idempotence)", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.State.AssertedModel = "master-model"

		if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if len(fx.Injector.calls) != 0 {
			t.Errorf("Injector.calls = %d; want zero (AssertedModel already matched the target)", len(fx.Injector.calls))
		}
		if fx.Deps.State.AssertedModel != "master-model" {
			t.Errorf("State.AssertedModel = %q; want unchanged %q", fx.Deps.State.AssertedModel, "master-model")
		}
	})

	t.Run("prompt write failure fires no injection and leaves AssertedModel unchanged", func(t *testing.T) {
		// The assertion is deliberately the LAST fallible act of BeginBatch:
		// an earlier failure must never leave the pane switched while the
		// caller's error path discards the unsaved AssertedModel mutation —
		// that divergence would silently skip a needed re-assertion later.
		fx := newBeginFixture(t)
		fx.Deps.State.AssertedModel = "some-other-model"
		// A regular file at the PromptsDir path makes MkdirAll — and thus the
		// prompt write — fail before the injection site is ever reached.
		blockedDir := filepath.Join(t.TempDir(), "prompts")
		if err := os.WriteFile(blockedDir, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("seed blocking file: %v", err)
		}
		fx.Deps.Geom.PromptsDir = blockedDir

		_, err := websterengine.BeginBatch(fx.Deps, 1)
		if err == nil {
			t.Fatal("BeginBatch() error = nil; want a prompt-write failure")
		}
		if len(fx.Injector.calls) != 0 {
			t.Errorf("Injector.calls = %d; want zero — a failed begin-batch must not have switched the pane", len(fx.Injector.calls))
		}
		if fx.Deps.State.AssertedModel != "some-other-model" {
			t.Errorf("State.AssertedModel = %q; want unchanged %q", fx.Deps.State.AssertedModel, "some-other-model")
		}
	})
}

// TestBeginBatch_PromptFilePrevDigest proves the fork prompt is written under PromptsDir with
// {{.prev_digest}} populated from the immediately preceding batch's persisted digest for batch N>1,
// and the first-batch sentinel when there is no preceding batch.
func TestBeginBatch_PromptFilePrevDigest(t *testing.T) {
	t.Run("batch 1 renders the first-batch sentinel", func(t *testing.T) {
		fx := newBeginFixture(t)

		result, err := websterengine.BeginBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		data, err := os.ReadFile(result.PromptPath)
		if err != nil {
			t.Fatalf("read prompt file %s: %v", result.PromptPath, err)
		}
		if !strings.Contains(string(data), "none (first batch)") {
			t.Errorf("prompt file does not contain the first-batch sentinel; got:\n%s", data)
		}
	})

	t.Run("batch N>1 renders the persisted predecessor digest", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{
			1: {
				Slug:     "json-flag",
				Terminal: true,
				Status:   "done",
				Digest: &websterengine.Digest{
					Batch:   "01-json-flag",
					Status:  websterengine.DigestStatusDone,
					HeadSHA: "deadbeef",
				},
			},
		}

		result, err := websterengine.BeginBatch(fx.Deps, 2)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		data, err := os.ReadFile(result.PromptPath)
		if err != nil {
			t.Fatalf("read prompt file %s: %v", result.PromptPath, err)
		}
		for _, want := range []string{"01-json-flag", "done", "head_sha=deadbeef"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("prompt file does not contain %q; got:\n%s", want, data)
			}
		}
	})

	t.Run("prompt path is under PromptsDir", func(t *testing.T) {
		fx := newBeginFixture(t)

		result, err := websterengine.BeginBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if filepath.Dir(result.PromptPath) != fx.PromptDir {
			t.Errorf("PromptPath dir = %q; want %q", filepath.Dir(result.PromptPath), fx.PromptDir)
		}
	})
}

// TestBeginBatch_StateUpdated proves BeginBatch mutates State exactly as documented: CurrentBatch
// and the fresh BatchState fields for the batch it began.
// There is no chain/restart concept left under the flat card-list model.
func TestBeginBatch_StateUpdated(t *testing.T) {
	fx := newBeginFixture(t)

	result, err := websterengine.BeginBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
	if fx.Deps.State.CurrentBatch != 1 {
		t.Errorf("State.CurrentBatch = %d; want 1", fx.Deps.State.CurrentBatch)
	}
	bs, ok := fx.Deps.State.Batches[1]
	if !ok {
		t.Fatal("State.Batches[1] missing after BeginBatch")
	}
	if bs.Slug != "json-flag" || bs.StartSHA != result.StartSHA || bs.Kind != "fork" || bs.SpawnedAt == "" {
		t.Errorf("State.Batches[1] = %+v; want Slug=json-flag StartSHA=%q Kind=fork SpawnedAt=<non-empty>", bs, result.StartSHA)
	}
}

// TestBeginBatch_CreatesReportsDir proves BeginBatch creates a missing reports dir itself: the fork
// writes its report there with whatever tool it likes — a plain shell redirect included, which
// never creates missing parents — and only the --fresh archive path recreated the dir before
// (crucible round fable-r1's F5: an ordinary first run left it absent and a shell-writing fork's
// report failed on ENOENT).
func TestBeginBatch_CreatesReportsDir(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.Geom.ReportsDir = filepath.Join(t.TempDir(), "reports")

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
	info, err := os.Stat(fx.Deps.Geom.ReportsDir)
	if err != nil || !info.IsDir() {
		t.Errorf("stat(reports dir) = %v, %v; want the dir created by BeginBatch", info, err)
	}
}

// TestBeginBatch_UnknownRoleErrors proves a missing role resolution fails loud rather than
// injecting a zero-value model, naming the missing role.
func TestBeginBatch_UnknownRoleErrors(t *testing.T) {
	fx := newBeginFixture(t)
	delete(fx.Deps.Roles, websterengine.RoleMaster)

	_, err := websterengine.BeginBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("BeginBatch() error = nil; want an error for a missing role resolution")
	}
	if !strings.Contains(err.Error(), string(websterengine.RoleMaster)) {
		t.Errorf("BeginBatch() error = %q; want it to name the missing role %q (batch %s)", err.Error(), websterengine.RoleMaster, strconv.Itoa(1))
	}
}

// TestBeginBatch_PreExistingReportRefused proves webster's own pre-existing-report guard applies to
// the fork path: a batch whose report file already exists is refused loud (naming the recovery escape)
// with its BatchState left untouched — finished work is never silently overwritten by an accidental
// re-begin.
// There is no --restart-chain escape under the flat card-list model.
func TestBeginBatch_PreExistingReportRefused(t *testing.T) {
	fx := newBeginFixture(t)
	reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{
		1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "done"},
	}

	_, err := websterengine.BeginBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("BeginBatch() with an existing report = nil error; want the pre-existing-report refusal")
	}
	if !strings.Contains(err.Error(), "recover-batch") {
		t.Errorf("error = %q; want it to name the recover-batch escape", err.Error())
	}
	if strings.Contains(err.Error(), "--restart-chain") {
		t.Errorf("error = %q; want no --restart-chain escape (chain machinery is gone)", err.Error())
	}
	if bs := fx.Deps.State.Batches[1]; !bs.Terminal || bs.Status != "done" {
		t.Errorf("Batches[1] = %+v; want the terminal done record untouched by the refusal", bs)
	}
}

// TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite proves F9's guard: when the batch being
// begun as a fork carries a prior recovery record whose strand the reed still reports live (a dead
// recovery keeps its substrate alive by design), BeginBatch stops that strand before the record
// overwrite erases its StrandGUID — otherwise the unreclaimed strand would race the fresh fork on
// the repo.
func TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.AssertedModel = "master-model" // skip the injector
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{
		1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "dead-but-live-recovery"},
	}
	fx.Reed.live = []string{"dead-but-live-recovery"}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch() error = %v; want nil", err)
	}

	if len(fx.Reed.removed) != 1 || fx.Reed.removed[0] != "dead-but-live-recovery" {
		t.Errorf("reed.removed = %v; want exactly [dead-but-live-recovery] stopped before the record overwrite", fx.Reed.removed)
	}
	// The record was overwritten to a fresh fork batch.
	if bs := fx.Deps.State.Batches[1]; bs.Kind != "fork" || bs.Terminal || bs.StrandGUID != "" {
		t.Errorf("Batches[1] = %+v; want a fresh non-terminal fork record with no strand", bs)
	}
}
