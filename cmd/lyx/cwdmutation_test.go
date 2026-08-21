// cwdmutation_test.go enforces the migrated half of the Cwd Resolution Invariant: an integration
// test file this task moved onto the cwd-context seam (RunCLIIn / lyxcwd.WithCwd) must never
// reintroduce a process-wide t.Chdir or os.Chdir call, in either spelling. This is the guard slice
// 15's discussion promised — machine-enforcing what would otherwise be review discipline only.
// See CONSTRAINTS.md's Cwd Resolution Invariant.
//
// This guard clones cmd/lyx/tierpurity_test.go's machinery: the exec.LookPath("go") clean skip, the
// go env GOMOD module-root resolution, filepath.WalkDir, the filepath.ToSlash normalisation before
// any comparison (Windows is the primary dev OS), and the report-every-violation-rather-than-the-
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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cwdMutationSubjectFiles is this guard's explicitly named per-file subject set (module-relative,
// slash-separated path -> true). The integration test files slice 15's migration touched, plus
// internal/fabricengine/coalesce_integration_test.go, the one file whose cwd mutation is itself the
// assertion under test rather than a removable seam-migration leftover.
// internal/loomengine/preflight_integration_test.go left this set when the preflight-loom-agnostic
// task deleted the file outright, retiring the whole suite rather than leaving a migrated leftover
// to track; internal/perchcli's two integration test files left it the same way when Retire-perch
// deleted the module.
var cwdMutationSubjectFiles = map[string]bool{
	"internal/fabriccli/cli_test.go":                     true,
	"internal/configcli/configcli_integration_test.go":   true,
	"internal/webstercli/verbs_test.go":                  true,
	"internal/idecli/cli_test.go":                        true,
	"internal/reedcli/cli_integration_test.go":           true,
	"internal/fabricengine/coalesce_integration_test.go": true,
}

// cwdMutationBannedTokens are the raw substrings a cwdMutationSubjectFiles entry may not contain,
// unless the file is on cwdMutationAllowlist. Both spellings are banned: banning only "t.Chdir("
// would miss the wrapper pattern several of these files carried before migration (restoreCwd,
// mustChdir), each of which called the bare "os.Chdir(" form instead.
var cwdMutationBannedTokens = []string{"t.Chdir(", "os.Chdir("}

// cwdMutationAllowlist is this guard's per-file allowlist (path module-relative, slash-separated ->
// reason). It carries exactly one entry: the file whose cwd mutation IS the assertion, not a
// migration leftover.
var cwdMutationAllowlist = map[string]string{
	"internal/fabricengine/coalesce_integration_test.go": `cwd is the assertion: TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd pins gitrepo.New("") against a non-git process cwd`,
}

// TestCwdMutation_MigratedFilesStayChdirFree walks the module tree and fails if any file on
// cwdMutationSubjectFiles (other than a cwdMutationAllowlist entry) contains t.Chdir( or os.Chdir( as
// a raw substring.
func TestCwdMutation_MigratedFilesStayChdirFree(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// tierpurity_test.go so this gate never blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	var scanned int
	var failures []string

	walkErr := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if tierPuritySkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, relErr := filepath.Rel(moduleRoot, path)
		if relErr != nil {
			return relErr
		}
		// Normalize to slash-separated form before any comparison.
		relPath = filepath.ToSlash(relPath)

		if !cwdMutationSubjectFiles[relPath] {
			return nil
		}
		scanned++

		if _, allowlisted := cwdMutationAllowlist[relPath]; allowlisted {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		if tok, found := firstCwdMutationToken(string(data)); found {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned cwd-mutation token %q -- a subject-set file must drive the module's RunCLIIn seam at an explicit cwd instead of moving the process (see CONSTRAINTS.md's Cwd Resolution Invariant), or add a cwdMutationAllowlist entry in cmd/lyx/cwdmutation_test.go with a reason",
				relPath, tok,
			))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

	// Vacuous-scan protection: every subject-set entry must actually be found on disk, or the
	// subject set (or a file path within it) has drifted.
	if scanned != len(cwdMutationSubjectFiles) {
		t.Fatalf("cwd mutation guard: scanned %d of %d subject files under %s; the subject set or a file path may have drifted", scanned, len(cwdMutationSubjectFiles), moduleRoot)
	}

	if len(failures) > 0 {
		t.Errorf("Cwd Resolution Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
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
	reason, allowlisted := cwdMutationAllowlist[allowlistedPath]
	if !allowlisted || reason == "" {
		t.Fatalf("%s missing a non-empty cwdMutationAllowlist reason", allowlistedPath)
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	data, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(allowlistedPath)))
	if err != nil {
		t.Fatalf("read %s: %v", allowlistedPath, err)
	}
	if _, found := firstCwdMutationToken(string(data)); !found {
		t.Fatalf("%s no longer contains a banned cwd-mutation token; its cwdMutationAllowlist entry is stale and should be removed", allowlistedPath)
	}
}
