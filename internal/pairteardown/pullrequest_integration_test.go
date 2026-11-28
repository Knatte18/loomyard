// pullrequest_integration_test.go covers the task branch a torn-down pair's pull request close names, over a real hub pair.

package pairteardown_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestClosePairPullRequest_NamesTheBareSlugBranchOfAPairMadeBeforeAPrefix pins that, with branch_prefix set over a live pair made under the empty prefix, the gone-worktree warning names the pair's bare slug.
// It captures the process-global logger, so it is not parallel.
func TestClosePairPullRequest_NamesTheBareSlugBranchOfAPairMadeBeforeAPrefix(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "pr-bare"
	hubforge.AddPair(t, h, slug)
	hubforge.SeedFabricConfig(t, h, "branch_prefix: hanf/\npathspec: \"\"\n")
	td, err := pairteardown.New(h.Location)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logs := logcapture.CaptureVerbose(t)

	pairteardown.ClosePairPullRequestForTest(td, slug)

	if !strings.Contains(logs.String(), "branch="+slug) || strings.Contains(logs.String(), "hanf/") {
		t.Errorf("log does not name the bare slug %q as the task branch, or names the prefixed branch:\n%s", slug, logs.String())
	}
}
