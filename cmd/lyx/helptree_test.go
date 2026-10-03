// helptree_test.go pins single help-tree behaviours; the module and subcommand listings are derived in clitree_test.go.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHelpTree_ConfigListsReconcile asserts that `lyx config --help` lists the reconcile verb.
func TestHelpTree_ConfigListsReconcile(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"config", "--help"}, &out); code != 0 {
		t.Fatalf("run([config --help]) = %d; want 0. output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "reconcile") {
		t.Errorf("config --help output missing %q; got:\n%s", "reconcile", out.String())
	}
}

// TestHelpTree_OrchStartListsAdopt asserts that `lyx orch start --help` lists the --adopt flag.
func TestHelpTree_OrchStartListsAdopt(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"orch", "start", "--help"}, &out); code != 0 {
		t.Fatalf("run([orch start --help]) = %d; want 0. output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "--adopt") {
		t.Errorf("orch start --help missing %q; got:\n%s", "--adopt", out.String())
	}
}
