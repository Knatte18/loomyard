//go:build integration

// livestate_helpers_test.go holds the one small helper the live-state harness's own files
// (livestate_states_test.go, livestate_verbs_test.go, livestate_manifest_selftest_test.go,
// livestate_refusal_selftest_test.go) call directly but that does not move with any of them:
// mustGit, copied verbatim from internal/hubforge/hub.go, since a state or a verb case needs to run a
// bare git command that must panic rather than fail a testing.TB it may not have in scope yet.

package fabricengine_test

import (
	"os/exec"
	"strings"
)

// mustGit runs a git subcommand in dir, panicking on non-zero exit.
// It is this harness's local equivalent to gitkit.MustRun and hubforge's own private copy: several
// state and verb builders run outside a point where a testing.TB is convenient to thread through, so it
// panics rather than taking one.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}
