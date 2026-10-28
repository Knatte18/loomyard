//go:build tier1

// replies_integration_test.go drives `lym report replies` through RunCLIIn over a hubforge hub: the prime-only refusal and the counts of a fake creel journal.

package reportcli

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestRepliesVerb replaces the process-global creelLog seam, so it does not run in parallel.
// It is a test of its own because TestReportVerbs_RefuseOutsideThePrimeThenReportTheSeededRun, which drives the other verbs over the same kind of hub, runs in parallel.
func TestRepliesVerb(t *testing.T) {
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	run := func(cwd string) (string, int) {
		var out bytes.Buffer
		code := RunCLIIn(cwd, &out, []string{"replies"})
		return out.String(), code
	}

	previous := creelLog
	t.Cleanup(func() { creelLog = previous })
	entries := []creelengine.LogEntry{
		{Event: "posted", ID: 1, From: "ly:orch", To: "ly:task:driver", ExpectReply: true},
		{Event: "posted", ID: 2, From: "ly:task:driver", To: "ly:orch", ReplyTo: 1},
		{Event: "chased", ID: 1, To: "ly:task:driver", Chase: 1},
	}
	var asked []*lymcwd.Location
	creelLog = func(location *lymcwd.Location) ([]creelengine.LogEntry, error) {
		asked = append(asked, location)
		return entries, nil
	}

	out, code := run(h.PairCodeWorktree(hubforge.TemplatePairSlug))
	if code != 1 {
		t.Errorf("from the pair's worktree: exit %d, output %s; want a refusal with exit 1", code, out)
	}
	envelope.RequireErr(t, out, "lym report replies")
	envelope.RequireErr(t, out, primeName)
	if len(asked) != 0 {
		t.Errorf("the journal was read %d times for a refused call; want none", len(asked))
	}

	out, code = run(h.PrimeWorktree())
	counted := envelope.RequireOK(t, out)
	if code != 0 {
		t.Fatalf("from the prime: exit %d, output %s; want exit 0", code, out)
	}
	// The marked post went to the driver, which answered it and was chased once; the orch received an unmarked reply and counts nowhere.
	want := []any{map[string]any{"role": "driver", "expected": 1.0, "replied": 1.0, "chased": 1.0, "unanswered": 0.0}}
	if roles, _ := counted.Raw["roles"].([]any); !reflect.DeepEqual(roles, want) {
		t.Errorf("roles = %v; want %v", roles, want)
	}
}
