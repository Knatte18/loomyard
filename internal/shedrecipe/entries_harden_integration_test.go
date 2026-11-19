// entries_harden_integration_test.go covers the HardenFix row's verify gate.
// The gate lints the overview, so the test runs over a git repository.

package shedrecipe

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHardenFixEntry_VerifyAsksTheHardenKey(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	env := hardenTestEnv(t)
	gitkit.Git(t, env.WorktreeRoot, "init", "-b", "main")
	gitkit.Git(t, env.WorktreeRoot, "config", "user.email", "test@test.com")
	gitkit.Git(t, env.WorktreeRoot, "config", "user.name", "Test")
	gitkit.CommitFile(t, env.WorktreeRoot, "a.txt", "a\n", "init")

	var commandKeys, scopeKeys []string
	env.VerifyCommand = func(key string) (string, error) { commandKeys = append(commandKeys, key); return "cmd", nil }
	env.VerifyMergeBase = func() (string, error) { return "", nil }
	env.VerifyTreeScope = func(key string) (string, bool, error) { scopeKeys = append(scopeKeys, key); return "", true, nil }

	gate := captureHardenFixGates(t, minimalFixConfig(), env)
	if result, err := gate[1].Gate(); err != nil || !result.Passed {
		t.Fatalf("verify gate = %+v, %v; want a pass over an empty tree-scoped command", result, err)
	}
	want := []string{"harden-burler-fix"}
	if !slices.Equal(commandKeys, want) || !slices.Equal(scopeKeys, want) {
		t.Errorf("VerifyCommand asked for %v and VerifyTreeScope for %v; want both for %v", commandKeys, scopeKeys, want)
	}
}
