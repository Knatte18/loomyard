// landingdeps_verify_test.go covers the three verify fields landingDeps tells landingshed:
// the VerifyCommand closure reads the plan at call time, and the two scratch paths match loomengine's accessors.
// It only writes files under t.TempDir(), so it stays Tier 1.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

const verifyOverviewHead = `---
format: 5
approved: true
---

# Plan: verify wiring

Framing paragraph.

## Card Index

1 — only — the only card
`

const verifyOverviewSection = `
## verify:

` + "```" + `
go build ./...
go test ./...
` + "```" + `
`

const verifyCardFile = "# Card 1 — only\n\n**Edit:**\n- `internal/x#Y`\n**Intent:** placeholder card.\n"

func writeVerifyPlan(t *testing.T, planDir, overview string) {
	t.Helper()
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "01-only.md"), []byte(verifyCardFile), 0o644); err != nil {
		t.Fatalf("write card: %v", err)
	}
}

func TestLandingDeps_VerifyCommandReadsPlanAtCallTime(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	deps := landingDeps(loc, websterengine.Geometry{StencilsDir: "/stencils"}, "task/foo",
		"https://example.com/origin.git", "main", true, func() error { return nil },
		modelspec.Registry{"claude/sonnet-5": {}}, &shuttleengine.Runner{}, landingshed.Config{})

	if _, err := deps.VerifyCommand(); err == nil {
		t.Fatal("VerifyCommand before any plan exists: want an error, got nil")
	}

	planDir := planparser.PlanDir(loc.AnchorPath())
	writeVerifyPlan(t, planDir, verifyOverviewHead+verifyOverviewSection)
	got, err := deps.VerifyCommand()
	if err != nil {
		t.Fatalf("VerifyCommand after plan written: %v", err)
	}
	if want := "go build ./... && go test ./..."; got != want {
		t.Errorf("VerifyCommand = %q, want %q", got, want)
	}

	writeVerifyPlan(t, planDir, verifyOverviewHead)
	got, err = deps.VerifyCommand()
	if err != nil || got != "" {
		t.Errorf("VerifyCommand with no verify section = (%q, %v), want (\"\", nil)", got, err)
	}

	if want := loomengine.LoomVerifyPendingPath(loc); deps.VerifyPendingPath != want {
		t.Errorf("VerifyPendingPath = %q, want %q", deps.VerifyPendingPath, want)
	}
	if want := loomengine.LoomVerifyOutputPath(loc); deps.VerifyOutputPath != want {
		t.Errorf("VerifyOutputPath = %q, want %q", deps.VerifyOutputPath, want)
	}
}
