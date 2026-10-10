//go:build llm

// smoke_seats_test.go is seatengine's opt-in live smoke: one chair and one advisor run against a real claude in real tmux panes.
// The chair asks the advisor one question over the session channel and writes the answer into its output.
// The test is the caller that wires the real substrate (reedengine, claudeengine and shuttleengine.Runner) in an external package, since seatengine itself never imports claudeengine (PATTERN-shuttle-provider-seam).
// It proves the seat machinery and the file contract, never answer quality, and is run by hand: it is compiled by every gate and run by none.

package seatengine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

const (
	smokeChairStencil   = "seatsmoke-chair"
	smokeAdvisorStencil = "seatsmoke-advisor"
	smokeRolePrefix     = "smoke"
)

// copyTestdataStencil copies testdata/<file> into stencilsDir as the stencil called name.
func copyTestdataStencil(t *testing.T, stencilsDir, name, file string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read testdata %s: %v", file, err)
	}
	path := stencilstore.Path(stencilsDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create stencil dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", name, err)
	}
}

func TestSmokeSeatsChairAsksAdvisor(t *testing.T) {
	llmkit.Claude(t, "LYX_REED_CLAUDE")

	h := hubforge.NewHub(t, ".")
	stencilsDir := fabricengine.StencilsDir(h.Location.HubPath)
	stencilkit.SeedInto(t, stencilsDir)
	copyTestdataStencil(t, stencilsDir, smokeChairStencil, "smoke-chair.md")
	copyTestdataStencil(t, stencilsDir, smokeAdvisorStencil, "smoke-advisor.md")

	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	reedCfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(h.Location.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	engine := seatengine.New(seatengine.RunnerShuttle(runner), seatengine.Geometry{
		WorktreeRoot: h.Location.WorktreePath(),
		AnchorPath:   h.Location.AnchorPath(),
		StencilsDir:  stencilsDir,
		ParentName:   reedGeom.ParentName,
		Shortname:    reedGeom.NameShortname,
		Slug:         reedGeom.NameSlug,
	})

	root := h.PrimeWorktree()
	advisorOutput := filepath.Join(root, "seat-smoke-advisor.md")
	chairOutput := filepath.Join(root, "seat-smoke-chair.md")
	table := seatengine.Table{
		RolePrefix: smokeRolePrefix,
		Segment:    segmentcolor.Plan,
		Timeout:    8 * time.Minute,
		Seats: []seatengine.Seat{
			{Name: seatengine.RoleChair, Stencil: smokeChairStencil, Inputs: []string{advisorOutput}, Outputs: []string{chairOutput}},
			{Name: seatengine.AdvisorName(1), Stencil: smokeAdvisorStencil, Outputs: []string{advisorOutput}},
		},
	}

	result, err := engine.Run(table)
	if err != nil {
		t.Fatalf("seat run: %v", err)
	}
	if result.Chair.Outcome != shuttleengine.OutcomeDone {
		t.Fatalf("chair outcome = %q; want %q; chair message: %q", result.Chair.Outcome, shuttleengine.OutcomeDone, result.Chair.LastAssistantMessage)
	}

	chairText, err := os.ReadFile(chairOutput)
	if err != nil {
		t.Fatalf("read chair output: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(chairText)), "oslo") {
		t.Errorf("chair output = %q; want it to name the advisor's answer, Oslo", chairText)
	}
	if _, err := os.Stat(advisorOutput); err != nil {
		t.Errorf("advisor output: %v; want the file written", err)
	}

	advisorStrand, err := agentname.Format(reedGeom.NameShortname, reedGeom.NameSlug, seatengine.SeatRole(smokeRolePrefix, seatengine.AdvisorName(1)))
	if err != nil {
		t.Fatalf("format advisor strand name: %v", err)
	}
	status, err := reedEngine.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, strand := range status.Strands {
		if strand.Name == advisorStrand {
			t.Errorf("advisor strand %q is still tracked after the chair's result (live=%v); want it stopped", advisorStrand, strand.Live)
		}
	}
}
