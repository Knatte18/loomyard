//go:build integration

// beginbatch_test.go exercises BeginBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine HeadSHA capture, while the model-injection
// seam (Injector) and the provider seam (shuttleengine.Engine) are local
// fakes, mirroring builderengine/spawn_test.go's fixture pattern. Plan
// values are constructed as *builderengine.Plan literals; the only
// directory ParsePlan-style parsing this file needs is Fingerprint's own
// *.md scan, backed by a t.TempDir() seeded with throwaway markdown files.

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newScratchRepo initializes a fresh git repo in a t.TempDir() and
// configures a throwaway committer identity, returning its path — mirrors
// builderengine/gitquery_test.go's helper of the same name (package-local;
// the two packages deliberately do not share a test-helper package).
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
// so builderengine.Fingerprint has something real to hash — BeginBatch's
// fingerprint gate reads planDir directly, never Plan.Batches, so no actual
// plan-format parsing is needed for these tests (per the card's own literal-
// Plan-value requirement).
func seedPlanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("seed plan dir: %v", err)
	}
	return dir
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

// ModelSwitchSequence returns a single marker PaneInput naming model, so a
// test asserting BeginBatch's Injector call can read the target model back
// out of the recorded inputs.
func (e *beginFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: "/model " + model, Submit: true}}
}

var _ shuttleengine.Engine = (*beginFakeEngine)(nil)

// beginFixture is a fully-wired set of BeginBatch dependencies: a real
// scratch git repo as WorktreeRoot, fresh webster/reports/prompts temp dirs,
// a literal two-batch plan (batch 1 plain, batch 2 oversized) backed by a
// seeded plan dir for the fingerprint gate, and every one of webster's three
// roles pre-resolved with distinct model names so a model-assertion test can
// tell them apart.
type beginFixture struct {
	Deps      websterengine.BeginDeps
	Injector  *beginFakeInjector
	Worktree  string
	PlanDir   string
	PromptDir string
}

func newBeginFixture(t *testing.T) *beginFixture {
	t.Helper()

	planDir := seedPlanDir(t)
	fingerprint, err := builderengine.Fingerprint(planDir)
	if err != nil {
		t.Fatalf("Fingerprint(%q) error = %v; want nil", planDir, err)
	}

	plan := &builderengine.Plan{
		Dir: planDir,
		Batches: []builderengine.PlanBatch{
			{Number: 1, Slug: "json-flag", File: "01-json-flag.md", Scope: []string{"internal/foo"}},
			{Number: 2, Slug: "list-tests", File: "02-list-tests.md", Oversized: true, Scope: []string{"internal/bar"}},
		},
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:          {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleMasterOversized: {Engine: "claude", Model: "oversized-model", Params: map[string]string{}},
		websterengine.RoleRecovery:        {Engine: "claude", Model: "recovery-model", Params: map[string]string{}},
	}

	injector := &beginFakeInjector{}
	promptsDir := t.TempDir()

	deps := websterengine.BeginDeps{
		Plan:         plan,
		State:        &websterengine.State{PlanFingerprint: fingerprint, MasterStrand: "master-strand-1"},
		Roles:        roles,
		Config:       websterengine.Config{SelfFixCap: 2},
		Engine:       &beginFakeEngine{},
		Injector:     injector,
		WorktreeRoot: worktree,
		WebsterDir:   t.TempDir(),
		ReportsDir:   t.TempDir(),
		PromptsDir:   promptsDir,
	}

	return &beginFixture{Deps: deps, Injector: injector, Worktree: worktree, PlanDir: planDir, PromptDir: promptsDir}
}

// TestBeginBatch_PauseSentinel proves the pause gate fires before anything
// else — including before the Injector is ever reached — and that the
// returned error satisfies errors.Is(err, websterengine.ErrPaused).
func TestBeginBatch_PauseSentinel(t *testing.T) {
	fx := newBeginFixture(t)

	if err := builderengine.RequestPause(fx.Deps.WebsterDir); err != nil {
		t.Fatalf("RequestPause() error = %v; want nil", err)
	}

	_, err := websterengine.BeginBatch(fx.Deps, 1, false)
	if !errors.Is(err, websterengine.ErrPaused) {
		t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPaused)", err)
	}
	if len(fx.Injector.calls) != 0 {
		t.Errorf("Injector was reached (%d calls) while paused; want zero", len(fx.Injector.calls))
	}
}

// TestBeginBatch_FingerprintMismatch proves a plan edited after run init is
// refused at begin-batch entry with the ErrFingerprintMismatch sentinel,
// before the Injector is ever reached.
func TestBeginBatch_FingerprintMismatch(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"

	_, err := websterengine.BeginBatch(fx.Deps, 1, false)
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

// TestBeginBatch_ModelAssertion proves the idempotent model-assertion rule:
// an oversized batch injects exactly once when AssertedModel still names the
// plain master model, updating AssertedModel afterward, while a repeat call
// once AssertedModel already names the target model injects zero times.
func TestBeginBatch_ModelAssertion(t *testing.T) {
	t.Run("oversized batch injects once and updates AssertedModel", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.State.AssertedModel = "master-model"

		result, err := websterengine.BeginBatch(fx.Deps, 2, false)
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
		if len(call.Inputs) != 1 || !strings.Contains(call.Inputs[0].Text, "oversized-model") {
			t.Errorf("Inject inputs = %v; want the oversized-model switch sequence", call.Inputs)
		}
		if fx.Deps.State.AssertedModel != "oversized-model" {
			t.Errorf("State.AssertedModel = %q; want %q", fx.Deps.State.AssertedModel, "oversized-model")
		}
		if result.AssertedModel != "oversized-model" {
			t.Errorf("BeginResult.AssertedModel = %q; want %q", result.AssertedModel, "oversized-model")
		}
	})

	t.Run("same-model batch injects zero times (idempotence)", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.State.AssertedModel = "oversized-model"

		if _, err := websterengine.BeginBatch(fx.Deps, 2, false); err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if len(fx.Injector.calls) != 0 {
			t.Errorf("Injector.calls = %d; want zero (AssertedModel already matched the target)", len(fx.Injector.calls))
		}
		if fx.Deps.State.AssertedModel != "oversized-model" {
			t.Errorf("State.AssertedModel = %q; want unchanged %q", fx.Deps.State.AssertedModel, "oversized-model")
		}
	})
}

// TestBeginBatch_PromptFilePrevDigest proves the fork prompt is written under
// PromptsDir with {{.prev_digest}} populated from the immediately preceding
// batch's persisted digest for batch N>1, and the first-batch sentinel when
// there is no preceding batch.
func TestBeginBatch_PromptFilePrevDigest(t *testing.T) {
	t.Run("batch 1 renders the first-batch sentinel", func(t *testing.T) {
		fx := newBeginFixture(t)

		result, err := websterengine.BeginBatch(fx.Deps, 1, false)
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
				Digest: &builderengine.Digest{
					Batch:        "01-json-flag",
					Status:       builderengine.DigestStatusDone,
					Tests:        "green",
					FilesChanged: 3,
				},
			},
		}

		result, err := websterengine.BeginBatch(fx.Deps, 2, false)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		data, err := os.ReadFile(result.PromptPath)
		if err != nil {
			t.Fatalf("read prompt file %s: %v", result.PromptPath, err)
		}
		for _, want := range []string{"01-json-flag", "done", "tests=green", "files_changed=3"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("prompt file does not contain %q; got:\n%s", want, data)
			}
		}
	})

	t.Run("prompt path is under PromptsDir", func(t *testing.T) {
		fx := newBeginFixture(t)

		result, err := websterengine.BeginBatch(fx.Deps, 1, false)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if filepath.Dir(result.PromptPath) != fx.PromptDir {
			t.Errorf("PromptPath dir = %q; want %q", filepath.Dir(result.PromptPath), fx.PromptDir)
		}
	})
}

// TestBeginBatch_StateUpdated proves BeginBatch mutates State exactly as
// documented: CurrentBatch and the fresh BatchState fields for the batch it
// began, and the chain-start SHA recorded once at the first chain member's
// begin-batch call, never overwritten by a later member's own call.
func TestBeginBatch_StateUpdated(t *testing.T) {
	fx := newBeginFixture(t)
	// Extend the fixture's plan with a two-member deferred-verify chain
	// (batch 3 declares chain-end 4; batch 4 IS the chain-end), mirroring
	// builderengine's own testdata/plan-valid chain shape.
	fx.Deps.Plan.Batches = append(fx.Deps.Plan.Batches, builderengine.PlanBatch{
		Number: 3, Slug: "refactor-a", File: "03-refactor-a.md", ChainEnd: 4,
	}, builderengine.PlanBatch{
		Number: 4, Slug: "refactor-b", File: "04-refactor-b.md",
	})

	result, err := websterengine.BeginBatch(fx.Deps, 1, false)
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

	if _, err := websterengine.BeginBatch(fx.Deps, 3, false); err != nil {
		t.Fatalf("BeginBatch(3) error = %v; want nil", err)
	}
	anchor, ok := fx.Deps.State.ChainStartSHAs[4]
	if !ok || anchor == "" {
		t.Fatalf("ChainStartSHAs[4] not recorded after begin-batch on chain member 3")
	}

	// Advance the host repo's HEAD before begin-batching the chain's other
	// member, so a wrongly-overwritten anchor would visibly differ.
	commitFile(t, fx.Worktree, "extra.txt", "extra", "extra commit")

	if _, err := websterengine.BeginBatch(fx.Deps, 4, false); err != nil {
		t.Fatalf("BeginBatch(4) error = %v; want nil", err)
	}
	if got := fx.Deps.State.ChainStartSHAs[4]; got != anchor {
		t.Errorf("ChainStartSHAs[4] = %q after begin-batching batch 4; want unchanged anchor %q", got, anchor)
	}
}

// TestBeginBatch_UnknownRoleErrors proves a batch whose target role has no
// resolved model-spec entry fails loud rather than injecting a zero-value
// model, naming the missing role. batchNumber is embedded in the message via
// strconv so the assertion does not depend on Roles map iteration order.
func TestBeginBatch_UnknownRoleErrors(t *testing.T) {
	fx := newBeginFixture(t)
	delete(fx.Deps.Roles, websterengine.RoleMaster)

	_, err := websterengine.BeginBatch(fx.Deps, 1, false)
	if err == nil {
		t.Fatal("BeginBatch() error = nil; want an error for a missing role resolution")
	}
	if !strings.Contains(err.Error(), string(websterengine.RoleMaster)) {
		t.Errorf("BeginBatch() error = %q; want it to name the missing role %q (batch %s)", err.Error(), websterengine.RoleMaster, strconv.Itoa(1))
	}
}
