// smoke_assist_test.go is burlerengine's opt-in live-integration smoke test for the assistant reviewer on Codex.
// TestSmokeBurlerReviewCodexAssistant drives one solo review round with a real Codex assistant on luna.
// The chair is a cheap Claude session; both run in real tmux panes.
// It mirrors smoke_round_test.go's tier gating, fixture hub, stencil seeding and stack wiring.
// It reuses that file's helpers (seedHubStencils, deferHubRelease, registerReedServer) rather than redefining them.
// The one difference in wiring is the engine set: shuttleengines.New builds every registered engine,
// so the chair resolves to claude and the assistant to codex.
// The assertions are mechanics only, never review quality.

package burlerengine_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSmokeBurlerReviewCodexAssistant drives one solo review round over the toy chair/table fixture.
// The chair is a cheap Claude reviewer (the stub chair), and the assistant is a Codex session on luna[low].
// It asserts the mechanics of the assisted round alone.
// The review half returns a verdict.
// ReviewResult.Assists holds one entry for seat "assist" with a non-empty Ending.
// A done assistant's report copy sits beside the review and parses with ParseCurrentReview.
// It skips unless the llm tier is selected, a claude binary resolves and a codex binary is on PATH.
// The operator runs it, capped at one run; it uses the operator's real Codex login through codex.yaml's default homes.
func TestSmokeBurlerReviewCodexAssistant(t *testing.T) {
	tiergate.Require(t, 3)
	llmkit.Claude(t, "LYM_REED_CLAUDE")
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex is not on PATH")
	}

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	seedHubStencils(t, h.Location.HubPath)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	targetPath := filepath.Join(h.PrimeWorktree(), "chair-table.txt")
	original := "In this small room there is a chair and a table. The chair is painted a " +
		"bright red color. The table is painted a deep blue color. Nothing else in " +
		"the room is described.\n"
	if err := os.WriteFile(targetPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write toy target file: %v", err)
	}
	reviewPath := filepath.Join(h.PrimeWorktree(), "burler-assist-smoke-review.md")

	profile := burlerengine.Profile{
		Target: burlerengine.FileSet{Paths: []string{targetPath}},
		Fasit: burlerengine.FileSet{
			Instructions: "the chair's color must match the table's color",
		},
		Rubric: "BLOCKING: the chair's color and the table's color, as described in the " +
			"target text, do not match.\nAPPROVED: the chair's color and the table's " +
			"color match; note anything else as a non-blocking MEDIUM/LOW/NIT finding.",
		FixScope:        burlerengine.FixScopeOverlay,
		ToolUse:         false,
		ReviewPath:      reviewPath,
		FixerReportPath: filepath.Join(h.PrimeWorktree(), "burler-assist-smoke-fixer-report.md"),
	}

	dirs := fabricengine.ConfigDirs(h.Location)
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	engines, err := shuttleengines.New(shuttleCfg, dirs)
	if err != nil {
		t.Fatalf("build engine set: %v", err)
	}
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	runner := shuttleengine.NewRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	burlerGeom, err := hubgeom.BurlerGeometry(h.Location)
	if err != nil {
		t.Fatalf("burler geometry: %v", err)
	}
	engine := burlerengine.New(seatengine.RunnerShuttle(runner), burlerGeom, lenses.Config{}, fabricengine.StencilsDir(h.Location.HubPath), "", nil)

	opts := burlerengine.RunOpts{
		Review:        shuttleengine.ModelChoice{Engine: "claude", Model: "haiku", Effort: "low"},
		Assist:        shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-luna", Effort: "low"},
		Round:         "1",
		Timeout:       8 * time.Minute,
		AssistTimeout: 5 * time.Minute,
	}
	review, err := engine.Review(profile, opts)
	if err != nil {
		t.Fatalf("burler review: %v", err)
	}
	if review.Verdict == "" {
		t.Errorf("review verdict is empty; want a verdict from the chair (outcome %q, chair message: %q)", review.Outcome, review.Review.LastAssistantMessage)
	}

	if len(review.Assists) != 1 {
		t.Fatalf("len(review.Assists) = %d; want 1", len(review.Assists))
	}
	assist := review.Assists[0]
	if assist.Seat != burlerengine.AssistSeatName {
		t.Errorf("assist seat = %q; want %q", assist.Seat, burlerengine.AssistSeatName)
	}
	if assist.Ending == "" {
		t.Fatalf("assist ending is empty; want the text of the assistant's end marker")
	}
	t.Logf("assistant ending = %q, report = %q, copy = %q", assist.Ending, assist.Report, assist.Copy)

	if assist.Ending != seatengine.EndingDone {
		return
	}
	copyPath := burlerengine.AssistCopyPath(reviewPath, burlerengine.AssistSeatName)
	if assist.Copy != copyPath {
		t.Errorf("assist copy = %q; want %q", assist.Copy, copyPath)
	}
	content, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("read the assistant's copy beside the review: %v", err)
	}
	if _, _, err := burlerengine.ParseCurrentReview(content); err != nil {
		t.Errorf("assistant's copy does not parse as a review: %v", err)
	}
}
