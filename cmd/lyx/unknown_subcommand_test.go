// unknown_subcommand_test.go pins the one behaviour of the root command not covered by clitree_test.go's walk.

package main

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestUpdateCommandRemoved verifies "lyx update" no longer resolves (folded into config reconcile).
func TestUpdateCommandRemoved(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"update"}, &out)

	if code != 1 {
		t.Errorf("run([update]) = %d; want 1 (update should be unknown)\noutput: %s", code, out.String())
	}

	envelope.RequireErr(t, out.String(), "")
}
