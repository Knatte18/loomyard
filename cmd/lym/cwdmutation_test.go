// cwdmutation_test.go guards the integration test files migrated onto the cwd-context seam (RunCLIIn / lymcwd.WithCwd).
// Each file on cwdMutationSubjectFiles must drive the RunCLIIn seam at an explicit cwd.
// It must never call t.Chdir or os.Chdir, in either spelling; this is the `PATTERN-cwd-resolution` invariant.
//
// The guard reports every violation rather than stopping at the first.
// Its subject set is an explicitly named per-file list, never a package prefix.
// A per-package subject would make the allowlist larger than the guarded set, which inverts the point of a guard.
// A file off the subject set is outside the guard entirely.
//
// A file joins cwdMutationSubjectFiles only once it is migrated onto the seam.
// An entry asserts that its chdir removal is complete and durable.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cwdMutationSubjectFiles is the explicitly named per-file subject set of this guard.
// Each entry is a module-relative, slash-separated path with the reason it belongs to the set.
var cwdMutationSubjectFiles = []scankit.Entry{
	{Key: "internal/fabriccli/cli_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/configcli/configcli_integration_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/webstercli/verbs_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/idecli/cli_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/reedcli/cli_integration_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/fabricengine/coalesce_integration_test.go", Why: "cwd mutation is the assertion under test"},
}

// cwdMutationBannedTokens are the raw substrings a guarded file may not contain, unless it is allowlisted.
// Both spellings are banned, because a wrapper around the bare "os.Chdir(" call would pass a check for "t.Chdir(" alone.
var cwdMutationBannedTokens = []string{"t.Chdir(", "os.Chdir("}

// cwdMutationAllowlist exempts subject-set files whose cwd mutation is itself the assertion under test.
// Its keys are module-relative, slash-separated paths, and each value is the reason.
var cwdMutationAllowlist = []scankit.Entry{
	{
		Key: "internal/fabricengine/coalesce_integration_test.go",
		Why: `cwd is the assertion: the coalesce-push test with an empty code-side path, run from an unrelated cwd, pins gitrepo.New("") against a non-git process cwd`,
	},
}

// TestCwdMutation_MigratedFilesStayChdirFree walks the module tree and fails on any file in cwdMutationSubjectFiles:
// a raw t.Chdir( or os.Chdir( substring fails the test unless the file is on cwdMutationAllowlist.
// A narrowed gate runs it only when a subject-set file or scankit changes; a whole gate always runs it.
//
//lym:guard
//lym:watch internal/fabriccli/cli_test.go internal/configcli/configcli_integration_test.go
//lym:watch internal/webstercli/verbs_test.go internal/idecli/cli_test.go internal/reedcli/cli_integration_test.go
//lym:watch internal/fabricengine/coalesce_integration_test.go internal/testkit/scankit/*.go
func TestCwdMutation_MigratedFilesStayChdirFree(t *testing.T) {
	tiergate.Require(t, 0)
	subjects := scankit.NewAllowlist(cwdMutationSubjectFiles)
	allow := scankit.NewAllowlist(cwdMutationAllowlist)
	var scanned int
	var failures []string

	scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		if !subjects.Allowed(f.Rel) {
			return
		}
		scanned++

		if allow.Allowed(f.Rel) {
			return
		}

		if tok, found := firstCwdMutationToken(string(f.Data)); found {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned cwd-mutation token %q -- a subject-set file must drive the module's RunCLIIn seam at an explicit cwd instead of moving the process (see `PATTERN-cwd-resolution`), or add a cwdMutationAllowlist entry in cmd/lym/cwdmutation_test.go with a reason",
				f.Rel, tok,
			))
		}
	})

	// Vacuous-scan protection: every subject-set entry must be found on disk.
	// A missing entry means the subject set, or a path within it, has drifted.
	scankit.RequireFloor(t, scanned, len(cwdMutationSubjectFiles), "cwd mutation guard")
	subjects.RequireNoStale(t)
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-cwd-resolution` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstCwdMutationToken returns the first banned token, in declared order, found as a raw substring of content.
// Its second result reports whether any was found.
func firstCwdMutationToken(content string) (string, bool) {
	for _, tok := range cwdMutationBannedTokens {
		if strings.Contains(content, tok) {
			return tok, true
		}
	}
	return "", false
}

// TestCwdMutationGuard_NotVacuous proves the matcher fires: a planted violation trips firstCwdMutationToken,
// and the one allowlisted file, read fresh from disk, still contains a banned token,
// so its silence comes from the allowlist branch.
// A narrowed gate runs it only when the allowlisted file or scankit changes; a whole gate always runs it.
//
//testtiming:keep proves the TestCwdMutation_MigratedFilesStayChdirFree guard fires on a planted violation and stays silent for its allowlisted file
//lym:guard
//lym:watch internal/fabricengine/coalesce_integration_test.go internal/testkit/scankit/*.go
func TestCwdMutationGuard_NotVacuous(t *testing.T) {
	tiergate.Require(t, 0)
	planted := "func TestPlanted(t *testing.T) {\n\tt.Chdir(t.TempDir())\n}\n"
	if tok, found := firstCwdMutationToken(planted); !found || tok != "t.Chdir(" {
		t.Fatalf("firstCwdMutationToken(planted) = (%q, %v); want (\"t.Chdir(\", true) -- the guard's matcher would be vacuous", tok, found)
	}

	const allowlistedPath = "internal/fabricengine/coalesce_integration_test.go"
	var reason string
	for _, e := range cwdMutationAllowlist {
		if e.Key == allowlistedPath {
			reason = e.Why
		}
	}
	if reason == "" {
		t.Fatalf("%s missing a non-empty cwdMutationAllowlist reason", allowlistedPath)
	}

	data, err := os.ReadFile(filepath.Join(scankit.Root(t), filepath.FromSlash(allowlistedPath)))
	if err != nil {
		t.Fatalf("read %s: %v", allowlistedPath, err)
	}
	if _, found := firstCwdMutationToken(string(data)); !found {
		t.Fatalf("%s no longer contains a banned cwd-mutation token; its cwdMutationAllowlist entry is stale and should be removed", allowlistedPath)
	}
}
