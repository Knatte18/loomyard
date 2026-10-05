// agentfiledissues_test.go enforces `PATTERN-agent-filed-issues`: lyx files a GitHub issue only through `lyx selfreport create`,
// run by an agent or the operator, so no production package outside the selfreport module calls selfreportengine.CreateIssue.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// agentFiledIssuesAllowlist names the package directories permitted to reference the issue-creating call:
// the selfreport CLI that backs `lyx selfreport create`, and the engine that declares it.
var agentFiledIssuesAllowlist = []scankit.Entry{
	{Key: "internal/selfreportcli/", Why: "the `lyx selfreport create` verb"},
	{Key: "internal/selfreportengine/", Why: "declares CreateIssue"},
}

// createIssueCall is the qualified call no other production package may contain.
const createIssueCall = "selfreportengine.CreateIssue"

// TestAgentFiledIssues_NoCreateIssueOutsideSelfreport walks every non-test *.go file under the module root
// and fails if a file outside the allowlisted selfreport directories contains selfreportengine.CreateIssue.
func TestAgentFiledIssues_NoCreateIssueOutsideSelfreport(t *testing.T) {
	allow := scankit.NewAllowlist(agentFiledIssuesAllowlist)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		if strings.Contains(string(f.Data), createIssueCall) {
			failures = append(failures, fmt.Sprintf(
				"%s: contains %s -- file an issue through `lyx selfreport create`, run by an agent or the operator",
				f.Rel, createIssueCall,
			))
		}
	})

	scankit.RequireFloor(t, scanned, 20, "agent-filed issues")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-agent-filed-issues` violated:\n%s", strings.Join(failures, "\n"))
	}
}
