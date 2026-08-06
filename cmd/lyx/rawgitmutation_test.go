// rawgitmutation_test.go closes the Fabric Git Invariant's tracked gap by machine-checking that internal/websterengine's and internal/builderengine's production source never construct a raw gitrepo handle or call gitexec.RunGit directly, beyond the two grandfathered read-only exemptions this file names.
// See CONSTRAINTS.md's Fabric Git Invariant (warp + weft) — the "Known gap, tracked" clause both packages' migration onto internal/fabricengine's warp-only methods closes.
//
// The guard bans the two CONSTRUCTION/CALL tokens (gitrepo.New(, gitexec.RunGit() — not per-verb method names: a verb-name ban would both flag the correctly-migrated consumer code (which legitimately calls .CheckoutDetached(/.ResetHard( on the new WarpBisector/WarpResetter interfaces) and miss the raw gitexec.RunGit( bypass that carries no method token at all.

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

// rawGitMutationScanPackages are the module-relative package subtrees this
// guard walks: exactly the two packages the discussion's regression-guard
// decision names, internal/websterengine and internal/builderengine.
var rawGitMutationScanPackages = []string{
	filepath.Join("internal", "websterengine"),
	filepath.Join("internal", "builderengine"),
}

// rawGitMutationBannedTokens are the raw substrings a non-test .go file in
// rawGitMutationScanPackages may not contain, unless the file is on
// rawGitMutationAllowlist.
var rawGitMutationBannedTokens = []string{
	"gitrepo.New(",
	"gitexec.RunGit(",
}

// rawGitMutationAllowlist is this guard's per-file allowlist (path module-
// relative, slash-separated → reason): the two grandfathered read-only
// exemptions the Fabric Git Invariant's "Known gap, tracked" clause carved
// out when the mutating paths in each package migrated onto
// internal/fabricengine's warp-only methods.
var rawGitMutationAllowlist = map[string]string{
	"internal/websterengine/gitwrap.go":  "grandfathered read-only exemptions — CurrentSHA via gitrepo.New and `status --porcelain` via gitexec.RunGit",
	"internal/builderengine/gitquery.go": "grandfathered read-only exemptions — HeadSHA/ChangedFiles/Dirty via gitexec.RunGit",
}

// rawGitMutationMinScannedFiles is the vacuous-scan floor for this guard's
// two-package walk: fewer than 4 production .go files found across both
// packages combined means the walk is misconfigured rather than either
// package having genuinely shrunk.
const rawGitMutationMinScannedFiles = 4

// TestNoRawGitMutation_WebsterBuilderProductionSource walks internal/websterengine's and internal/builderengine's non-test .go files and fails if any of them (other than a rawGitMutationAllowlist entry) contains the raw substring "gitrepo.New(" or "gitexec.RunGit(" — the two construction/call tokens a raw, fabric-bypassing git mutation would carry.
func TestNoRawGitMutation_WebsterBuilderProductionSource(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH,
	// mirroring tierpurity_test.go and hermeticenv_test.go so this gate
	// never blocks a minimal environment.
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

	for _, pkgRel := range rawGitMutationScanPackages {
		pkgDir := filepath.Join(moduleRoot, pkgRel)

		walkErr := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			relPath, relErr := filepath.Rel(moduleRoot, path)
			if relErr != nil {
				return relErr
			}
			// Normalize to slash-separated form before any comparison,
			// exactly as tierpurity_test.go does: filepath.WalkDir yields
			// backslash paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			scanned++

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(data)

			if _, allowlisted := rawGitMutationAllowlist[relPath]; allowlisted {
				return nil
			}

			for _, tok := range rawGitMutationBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains banned raw-git-mutation token %q — mutating git in this package must dispatch through internal/fabricengine's warp-only methods (see CONSTRAINTS.md's Fabric Git Invariant), or add a rawGitMutationAllowlist entry in cmd/lyx/rawgitmutation_test.go with a reason if this is a new grandfathered read-only exemption",
						relPath, tok,
					))
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("failed to walk %s: %v", pkgDir, walkErr)
		}
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < rawGitMutationMinScannedFiles {
		t.Fatalf("raw git mutation guard: only scanned %d production .go file(s) across %v; expected at least %d — the walk may be misconfigured", scanned, rawGitMutationScanPackages, rawGitMutationMinScannedFiles)
	}

	if len(failures) > 0 {
		t.Errorf("Fabric Git Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}
