// beginbatch_helper_test.go holds the BeginBatch fixture and the C# re-sign plan the tier0 and tier1 begin-batch and record-batch tests share.
// The fixtures are package-local: the internal and external test packages deliberately do not share a test-helper package.

package websterengine_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// csharpRendererSource is a C# file declaring one method, Draw, taking one parameter of paramType.
func csharpRendererSource(paramType string) string {
	return fmt.Sprintf("namespace Glyphchain.Sharp\n{\n    public class Renderer\n    {\n        public void Draw(%s count)\n        {\n        }\n    }\n}\n", paramType)
}

// csharpResignPlanDir writes a two-card csharp plan into a fresh directory and returns the directory with the parsed plan.
// Card 1 re-signs Glyphchain.Sharp#Renderer.Draw(int) to take a long, and card 2 names the old glyph under Uses.
func csharpResignPlanDir(t *testing.T) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()
	prosa := []plankit.Group{{Label: "Prosa", Targets: []string{"csharp/Renderer.cs"}}}
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "csharp",
		Framing:  "A two-card fixture whose first card re-signs a member that its second card names.",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "json-flag",
				Summary: "re-sign Draw",
				Groups:  []plankit.Group{{Label: "Edit", Targets: []string{"Glyphchain.Sharp#Renderer", "Glyphchain.Sharp#Renderer.Draw(int)` -> `public void Draw(long count)"}}},
				Intent:  "Re-sign Draw to take a long, a change that modifies the Renderer type's body too.",
			},
			{
				Number:  2,
				Slug:    "pending",
				Summary: "the not-yet-built card",
				Groups:  prosa,
				Uses:    []string{"Glyphchain.Sharp#Renderer.Draw(int)"},
				Intent:  "The pending card that names Draw by its old glyph.",
			},
		},
	})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	return dir, plan
}

// beginCard returns a minimal single-card batcher.Batch identifying number and slug.
// It rests on begin-batch's batchIdentity assumption: batch ≡ card under the identity batchifier.
func beginCard(number int, slug string) batcher.Batch {
	return batcher.Batch{Cards: []planparser.Card{{Number: number, Slug: slug, Title: slug, Intent: "placeholder card " + slug}}}
}

// beginFixture is a fully-wired set of BeginBatch dependencies.
// It holds a temp directory with base.txt as WorktreeRoot over a fakeGit at one commit, fresh webster/reports/prompts temp dirs, and two literal single-card execution batches backed by a seeded plan dir for the fingerprint gate.
type beginFixture struct {
	Deps      websterengine.BeginDeps
	Stopper   *recordingStopper
	Git       *fakeGit
	Worktree  string
	PlanDir   string
	PromptDir string
}

func newBeginFixture(t *testing.T) *beginFixture {
	t.Helper()

	planDir := seedPlanDir(t)
	fp := mustFingerprint(t, planDir)

	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		t.Fatalf("ParsePlan(%q): %v", planDir, err)
	}
	batches := []batcher.Batch{
		beginCard(1, "json-flag"),
		beginCard(2, "list-tests"),
	}

	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	index := newFakeIndex()

	promptsDir := t.TempDir()
	stopper := &recordingStopper{}

	// webster's prompts are read from disk at call time, so the fixture's hub must carry them before BeginBatch reaches RenderForkPrompt.
	hubPath := filepath.Dir(worktree)
	seedHubStencils(t, hubPath)

	deps := websterengine.BeginDeps{
		Plan:    plan,
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: fp, MasterStrand: "master-strand-1"},
		Config:  websterengine.Config{SelfFixCap: 2},
		Stopper: stopper,
		Gate:    testGateConfig(),
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   promptsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			SpecsDir:     fabricengine.SpecsDir(hubPath),
			PlanDir:      planDir,
			Git:          git,
			Index:        index,
		},
	}

	return &beginFixture{Deps: deps, Stopper: stopper, Git: git, Worktree: worktree, PlanDir: planDir, PromptDir: promptsDir}
}
