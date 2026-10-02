//go:build integration

// parentreview_integration_test.go drives the whole parent-review exchange against a real hub pair:
// the Discussion-Write row's gate list as wire() builds it, the real verbs through RunCLIIn, the real round store, status, and the discussion commit.
// Only the shuttle is scripted.
// The fake replays arrivals against the row's real gate list the way the wait loop does at a turn boundary,
// so the test pins how the closures, the store and the verbs fit together, not the wait loop itself (internal/shuttleengine's own tests cover that).

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	prReviewParent = "hub:orchestrator"
	prValidRecord  = "# Decision Record\n\n" +
		"## Goal\nExercise the parent review.\n\n" +
		"## Scope\nOne gated Discussion-Write row.\n\n" +
		"## Decisions\nLet the parent review the discussion.\n\n" +
		"## Constraints\nNone beyond the harness.\n\n" +
		"## Auto-mode assumptions\nNone.\n\n" +
		"## Open risks\nNone.\n\n" +
		"## Acceptance criteria\nThe gate lets the run through.\n"
)

// scriptedShuttle is a shedadapters.Shuttle whose gated run is a test-supplied script.
// The producer's attach probe finds nothing, so every Call takes the fresh-spawn branch and runs the script once.
type scriptedShuttle struct {
	script func(gates shuttleengine.GateSpec) shuttleengine.Result
}

func (s *scriptedShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	panic("scriptedShuttle: the producer must run gated")
}

func (s *scriptedShuttle) Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	panic("scriptedShuttle: the producer must attach gated")
}

func (s *scriptedShuttle) AttachGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, bool, error) {
	return shuttleengine.Result{}, false, nil
}

func (s *scriptedShuttle) RunGated(_ shuttleengine.Spec, gates shuttleengine.GateSpec) (shuttleengine.Result, error) {
	return s.script(gates), nil
}

// arrive evaluates the gate list once, in order, the way the wait loop does at a turn boundary:
// the first entry that is pending or failing stops the evaluation and is returned.
func arrive(t *testing.T, gates shuttleengine.GateSpec) shuttleengine.GateResult {
	t.Helper()
	for _, g := range gates {
		if g.Attempts == 0 {
			continue
		}
		res, err := g.Gate()
		if err != nil {
			t.Fatalf("gate %q = error %v; want nil", g.Name, err)
		}
		if res.Pending || !res.Passed {
			return res
		}
	}
	return shuttleengine.GateResult{Passed: true}
}

// prFixture is a hub pair wired the way a real loom run wires it, with the producer built from the embedded recipe's Discussion-Write row.
type prFixture struct {
	t        *testing.T
	hub      *hubforge.Hub
	slug     string
	location *lyxcwd.Location
	weft     string
	shuttle  *scriptedShuttle
	producer shedengine.ShedProducer
	store    parentreview.Store
}

func newPRFixture(t *testing.T) *prFixture {
	t.Helper()
	hub := hubforge.NewHub(t, ".")
	const slug = "parentreview"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree = %v; want nil", err)
	}
	if err := shedrun.WriteSeed(location, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo, Parent: prReviewParent}); err != nil {
		t.Fatalf("WriteSeed = %v; want nil", err)
	}

	// stencilstore.Read hard-errors on a missing file, so the two parent-review stencils are deployed into the hub's board by hand.
	loomStencils := filepath.Join(fabricengine.StencilsDir(hub.Path), "loom")
	if err := os.MkdirAll(loomStencils, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"loom-template-parent-review-delivery.md": stencils.LoomTemplateParentReviewDelivery,
		"loom-template-parent-review-brief.md":    stencils.LoomTemplateParentReviewBrief,
	} {
		if err := os.WriteFile(filepath.Join(loomStencils, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire = %v; want nil", err)
	}
	f := &prFixture{t: t, hub: hub, slug: slug, location: location, weft: hub.PairWeftSibling(slug), shuttle: &scriptedShuttle{}, store: reviewStoreFor(location)}

	decision, support := loomengine.DiscussionDecisionRecord(location), loomengine.DiscussionSupportLog(location)
	env := c.env
	env.Shuttle = f.shuttle
	env.DiscussionSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{Prompt: "write the discussion", OutputFiles: []string{decision, support}}, nil
	}

	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse = %v; want nil", err)
	}
	for _, row := range recipe.Producers {
		if row.Name != "Discussion-Write" {
			continue
		}
		construct, err := shedrecipe.Lookup(row.Engine)
		if err != nil {
			t.Fatalf("shedrecipe.Lookup(%q) = %v; want nil", row.Engine, err)
		}
		if f.producer, err = construct(row.Name, row.Config, env); err != nil {
			t.Fatalf("build Discussion-Write = %v; want nil", err)
		}
	}
	if f.producer == nil {
		t.Fatal("no Discussion-Write row in the embedded recipe")
	}
	return f
}

// writeDiscussion writes a valid discussion; extra distinguishes a revision from the first draft.
func (f *prFixture) writeDiscussion(extra string) {
	f.t.Helper()
	writeRecordFile(f.t, loomengine.DiscussionDecisionRecord(f.location), prValidRecord+extra)
	writeRecordFile(f.t, loomengine.DiscussionSupportLog(f.location), "support log\n")
}

// verb runs one `lyx loom` verb from cwd and returns its exit code and output.
func (f *prFixture) verb(cwd string, args ...string) (int, string) {
	f.t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, args)
	return code, out.String()
}

func (f *prFixture) latest() parentreview.Round {
	f.t.Helper()
	r, ok, err := f.store.Latest()
	if err != nil || !ok {
		f.t.Fatalf("Latest = (%v, %v); want a round", ok, err)
	}
	return r
}

// call runs the producer once and requires Done.
func (f *prFixture) call() {
	f.t.Helper()
	outcome, _, err := f.producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		f.t.Fatalf("Discussion-Write Call = (%q, %v); want done", outcome, err)
	}
}

// TestParentReviewExchange_RejectThenFixThenLetThrough drives round 1 through delivery, a reject from the prime, the writer's fix and the let-through,
// then round 2 through an approve, and checks the commit, the status wait and the round numbering along the way.
func TestParentReviewExchange_RejectThenFixThenLetThrough(t *testing.T) {
	f := newPRFixture(t)
	taskCwd := f.location.AnchorPath()
	primeCwd := f.hub.Location.AnchorPath()
	reviewFile := filepath.Join(t.TempDir(), "review.md")
	const findings = "The goal section names no acceptance test.\n"

	writeStatusFile(t, f.location, `{"current_producer":"Discussion-Write","state":"running"}`)

	f.shuttle.script = func(gates shuttleengine.GateSpec) shuttleengine.Result {
		f.writeDiscussion("")

		// Arrival 1: the discussion passes, the request opens and the delivery prompt names the parent and the request.
		first := arrive(t, gates)
		round := f.latest()
		if !first.Pending || !strings.Contains(first.Send, prReviewParent) || !strings.Contains(first.Send, round.RequestPath()) {
			t.Fatalf("arrival 1 = %+v; want pending carrying a prompt naming %q and %q", first, prReviewParent, round.RequestPath())
		}
		if round.Number != 1 {
			t.Fatalf("round number = %d; want 1", round.Number)
		}

		// Status shows the wait while the request is open.
		code, out := f.verb(taskCwd, "status")
		var status map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &status); err != nil || code != 0 {
			t.Fatalf("status = (%d, %q); want a JSON envelope and exit 0", code, out)
		}
		if waiting, _ := status["waiting"].(string); !strings.HasPrefix(waiting, "parent review") {
			t.Errorf("status waiting = %v; want a note naming the parent review", status["waiting"])
		}

		// The writer's pane delivered the notice; the parent rejects from the prime.
		if code, out := f.verb(taskCwd, "review", "delivered"); code != 0 {
			t.Fatalf("review delivered = %d %q; want 0", code, out)
		}
		if err := os.WriteFile(reviewFile, []byte(findings), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, out := f.verb(primeCwd, "review", "reject", f.slug, reviewFile); code != 0 {
			t.Fatalf("review reject = %d %q; want 0", code, out)
		}

		// Arrival 2: the findings name the copied review file, and the writer is re-prompted once.
		second := arrive(t, gates)
		if second.Passed || second.Pending || !strings.Contains(second.Findings, round.ReviewPath()) {
			t.Fatalf("arrival 2 = %+v; want a failure whose findings name %q", second, round.ReviewPath())
		}
		if got, err := os.ReadFile(round.ReviewPath()); err != nil || string(got) != findings {
			t.Fatalf("copied review = (%q, %v); want the reviewer's findings", got, err)
		}

		// Arrival 3: the writer fixed the discussion; discussion re-runs and the parent review lets it through without a second request.
		f.writeDiscussion("\nAddressed the parent's finding.\n")
		third := arrive(t, gates)
		if !third.Passed || third.Pending || third.Send != "" {
			t.Fatalf("arrival 3 = %+v; want passed with nothing to send", third)
		}
		if got := f.latest(); got.Number != 1 {
			t.Fatalf("round number after the fix = %d; want still 1", got.Number)
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true}}
	}
	f.call()

	// The request directory rode along in the discussion commit.
	requestRel := filepath.ToSlash(filepath.Join(loomengine.LoomParentReviewDirRel(), "round-1", "request.json"))
	if got := mustGitOut(t, f.weft, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(got, requestRel) {
		t.Errorf("weft HEAD touched %q; want it to include %q", got, requestRel)
	}

	// A fresh start after the completed round opens round 2, and an approve lets the run through with no re-prompt.
	f.shuttle.script = func(gates shuttleengine.GateSpec) shuttleengine.Result {
		f.writeDiscussion("")
		first := arrive(t, gates)
		round := f.latest()
		if !first.Pending || first.Send == "" || round.Number != 2 {
			t.Fatalf("round 2 arrival 1 = %+v in round %d; want pending with a prompt in round 2", first, round.Number)
		}
		if code, out := f.verb(taskCwd, "review", "approve"); code != 0 {
			t.Fatalf("review approve = %d %q; want 0", code, out)
		}
		second := arrive(t, gates)
		if !second.Passed || second.Pending || second.Send != "" {
			t.Fatalf("round 2 arrival 2 = %+v; want passed with no re-prompt", second)
		}
		if v := f.latest().Verdict; v == nil || v.Kind != parentreview.VerdictApprove {
			t.Fatalf("round 2 verdict = %+v; want approve", v)
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true}}
	}
	f.call()

	round2Rel := filepath.ToSlash(filepath.Join(loomengine.LoomParentReviewDirRel(), "round-2", "request.json"))
	if got := mustGitOut(t, f.weft, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(got, round2Rel) {
		t.Errorf("weft HEAD touched %q; want it to include %q", got, round2Rel)
	}
}
