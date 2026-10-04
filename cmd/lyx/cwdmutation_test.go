// cwdmutation_test.go enforces the migrated half of the Cwd Resolution Invariant: an integration
// test file this task moved onto the cwd-context seam (RunCLIIn / lyxcwd.WithCwd) must never
// reintroduce a process-wide t.Chdir or os.Chdir call, in either spelling. This is the guard slice
// 15's discussion promised — machine-enforcing what would otherwise be review discipline only.
// See `PATTERN-cwd-resolution`.
//
// This guard walks through scankit and keeps tierpurity_test.go's report-every-violation-rather-than-the-
// first posture. It departs from tierpurity_test.go in one respect: the subject set here is an
// explicitly named per-file list, never a package prefix. Eleven packages gained a seam change in
// this task, but each carries further non-smoke chdir-using test files this task deliberately did
// not touch, plus twelve deferred smoke files -- a per-package subject would make the allowlist
// larger than the guarded set, which inverts the point of a guard. Only a file on
// cwdMutationSubjectFiles is scanned at all; every file off it carries no allowlist entry and is
// outside the guard entirely, so the guard stays silent about work this task chose not to do.
//
// Growth rule: a file joins cwdMutationSubjectFiles only when it is migrated onto the seam, never by
// default. Adding an entry here asserts that file's chdir removal is complete and durable, not
// merely convenient today.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// cwdMutationSubjectFiles is this guard's explicitly named per-file subject set (module-relative,
// slash-separated path -> true). The integration test files slice 15's migration touched, plus
// internal/fabricengine/coalesce_integration_test.go, the one file whose cwd mutation is itself the
// assertion under test rather than a removable seam-migration leftover.
// internal/loomengine/preflight_integration_test.go left this set when the preflight-loom-agnostic
// task deleted the file outright, retiring the whole suite rather than leaving a migrated leftover
// to track, and two more integration test files left it the same way when the module owning them
// was retired.
var cwdMutationSubjectFiles = []scankit.Entry{
	{Key: "internal/fabriccli/cli_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/configcli/configcli_integration_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/webstercli/verbs_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/idecli/cli_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/reedcli/cli_integration_test.go", Why: "migrated onto the RunCLIIn seam"},
	{Key: "internal/fabricengine/coalesce_integration_test.go", Why: "cwd mutation is the assertion under test"},
}

// cwdMutationBannedTokens are the raw substrings a cwdMutationSubjectFiles entry may not contain,
// unless the file is on cwdMutationAllowlist. Both spellings are banned: banning only "t.Chdir("
// would miss the wrapper pattern several of these files carried before migration (restoreCwd,
// mustChdir), each of which called the bare "os.Chdir(" form instead.
var cwdMutationBannedTokens = []string{"t.Chdir(", "os.Chdir("}

// cwdMutationAllowlist is this guard's per-file allowlist (path module-relative, slash-separated ->
// reason). It carries exactly one entry: the file whose cwd mutation IS the assertion, not a
// migration leftover.
var cwdMutationAllowlist = []scankit.Entry{
	{
		Key: "internal/fabricengine/coalesce_integration_test.go",
		Why: `cwd is the assertion: the coalesce-push test with an empty code-side path, run from an unrelated cwd, pins gitrepo.New("") against a non-git process cwd`,
	},
}

// TestCwdMutation_MigratedFilesStayChdirFree walks the module tree and fails if any file on
// cwdMutationSubjectFiles (other than a cwdMutationAllowlist entry) contains t.Chdir( or os.Chdir( as
// a raw substring.
func TestCwdMutation_MigratedFilesStayChdirFree(t *testing.T) {
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
				"%s: contains banned cwd-mutation token %q -- a subject-set file must drive the module's RunCLIIn seam at an explicit cwd instead of moving the process (see `PATTERN-cwd-resolution`), or add a cwdMutationAllowlist entry in cmd/lyx/cwdmutation_test.go with a reason",
				f.Rel, tok,
			))
		}
	})

	// Vacuous-scan protection: every subject-set entry must actually be found on disk, or the
	// subject set (or a file path within it) has drifted.
	scankit.RequireFloor(t, scanned, len(cwdMutationSubjectFiles), "cwd mutation guard")
	subjects.RequireNoStale(t)
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-cwd-resolution` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstCwdMutationToken returns the first entry of cwdMutationBannedTokens (in declared order) that
// appears as a raw substring of content, and whether any was found.
func firstCwdMutationToken(content string) (string, bool) {
	for _, tok := range cwdMutationBannedTokens {
		if strings.Contains(content, tok) {
			return tok, true
		}
	}
	return "", false
}

// TestCwdMutationGuard_NotVacuous proves this guard's matcher actually fires, mirroring how
// tierpurity_test.go carries its own banned tokens as test data: a planted violation string trips
// firstCwdMutationToken, and the one real allowlisted file — which genuinely still contains a banned
// token, read fresh from disk rather than assumed — is proven to stay silent through the allowlist
// branch, not through an accidental absence of the token it exists to exempt.
func TestCwdMutationGuard_NotVacuous(t *testing.T) {
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
