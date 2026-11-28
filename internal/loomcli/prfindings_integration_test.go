// prfindings_integration_test.go drives the pr-findings verb through the real cobra tree against a real hub pair.
// It pins what prfindings_test.go's fake edge cannot:
// the command wires the run's landing seams before it builds the shed.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// blockingFindingsFile is a findings file in burler's review format holding one BLOCKING finding.
const blockingFindingsFile = `---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: main.go:1
    summary: the change is wrong
    fix: change it
---

### [BLOCKING] The change is wrong

**Location:** main.go:1

**Issue:** The change is wrong.
`

// TestPRFindingsCLI_SendsBackOnATapestryRun asserts that the verb builds the run's shed with its landing rows wired.
// On a tapestry run with the template's budgets, the PR-Review row's reroute to Code-Bouncer can fire,
// so a BLOCKING finding goes back to Code-Review.
func TestPRFindingsCLI_SendsBackOnATapestryRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	worktree := hub.PairCodeWorktree(hubforge.TemplatePairSlug)
	location, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	stencilkit.SeedInto(t, fabricengine.StencilsDir(location.HubPath))
	err = pullrequest.WriteRecord(
		shedrun.PullRequestRecord(location, shedrun.SelfRunID),
		shedrun.PullRequestRecordLock(location, shedrun.SelfRunID),
		pullrequest.Record{},
	)
	if err != nil {
		t.Fatalf("WriteRecord error = %v; want nil", err)
	}
	findings := filepath.Join(worktree, ".scratch", "pr-findings.md")
	if err := os.MkdirAll(filepath.Dir(findings), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v; want nil", err)
	}
	if err := os.WriteFile(findings, []byte(blockingFindingsFile), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v; want nil", err)
	}

	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), worktree, &out, []string{"pr-findings", findings})
	if code != 0 {
		t.Fatalf("exit = %d, out %s; want 0", code, out.String())
	}
	env := envelope.RequireOK(t, out.String())
	if got := env.Raw["send_back"]; got != true {
		t.Errorf("send_back = %v; want true, in %s", got, out.String())
	}
}
