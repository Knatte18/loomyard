// destructiveguard_test.go closes slice 12's completeness proof by machine-checking that
// internal/fabricengine's production source contains no destructive primitive call outside
// destroy.go — the one file the Fabric Destruction Chokepoint Invariant names as permitted to
// perform one.
// See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant.
//
// This guard clones cmd/lyx/rawgitmutation_test.go's machinery wholesale: the module-relative
// scan-package list, the raw-substring banned-token slice, the per-file allowlist map keyed by
// module-relative slash-separated path with a reason as its value, the minimum-scanned-files
// floor, the exec.LookPath("go") clean skip, the go env GOMOD module-root resolution, the
// filepath.WalkDir skipping of test files, and the filepath.ToSlash normalisation before any
// comparison, which matters because Windows is the primary dev OS.
//
// Two of the eight banned tokens were corrected against a naive first guess in opposite
// directions, and the reasons are recorded here because both mistakes are easy to reintroduce.
//
// "RemoveAll(" rather than "os.RemoveAll(": the package's removal seam (destroy.go's
// `var RemoveAll = os.RemoveAll`) is called bare at its call sites, so the qualified spelling
// "os.RemoveAll(" is not a substring of a bare `RemoveAll(hubPath)` call and would miss the two
// sites the slice most wants policed, one of them the hub teardown. The bare form "RemoveAll(" is
// a superset that also catches the qualified "os.RemoveAll(" form, and the seam's own declaration
// carries no trailing paren so it does not self-flag.
//
// "warp.ResetHard(" / "weft.ResetHard(" rather than ".ResetHard(": the broad ".ResetHard(" form
// would flag the *correctly migrated* callers, since the gated reset is reached as a method call
// on the pair handle (e.g. `f.warp.ResetHard(sha)` inside destroy.go itself, or a future
// `weft`-side caller). Banning the raw handles instead targets what is actually forbidden —
// reaching past the gate to the underlying repo field — and needs no leading dot, so it matches
// under any receiver name.
//
// The allowlist is per-file, so a *new* raw removal added inside an allowlisted file is not
// caught by this guard — the same limitation the file being cloned already has. The junction.go
// and hook.go entries in particular are whole-file allowlists for exactly one already-audited
// call each, not a blanket exemption for future removals in those files.

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

// destructiveGuardScanPackages are the module-relative package subtrees this guard walks: exactly
// the one package the discussion's bypass-guard decision names, internal/fabricengine.
var destructiveGuardScanPackages = []string{
	filepath.Join("internal", "fabricengine"),
}

// destructiveGuardBannedTokens are the raw substrings a non-test .go file in
// destructiveGuardScanPackages may not contain, unless the file is on destructiveGuardAllowlist.
// This is the discussion's final seven tokens plus "createdToken{", added per the overview's
// decision that the token's unforgeability is guard-enforced rather than type-enforced.
var destructiveGuardBannedTokens = []string{
	"RemoveAll(",
	"os.Remove(",
	`"worktree", "remove"`,
	`"branch", "-D"`,
	"warp.ResetHard(",
	"weft.ResetHard(",
	"fslink.Remove(",
	"createdToken{",
}

// destructiveGuardAllowlist is this guard's per-file allowlist (path module-relative,
// slash-separated → reason). Every entry carries the reason its one audited destructive call
// site is safe, per the Fabric Destruction Chokepoint Invariant's requirement that every
// allowlist entry name one.
var destructiveGuardAllowlist = map[string]string{
	"internal/fabricengine/destroy.go": "the gate's own file — the one file the invariant permits to perform a destructive primitive",
	"internal/fabricengine/gitexclude.go": "writeFileAtomically's os.Remove(tempPath) cleans up a temp file the same function created " +
		"under a repo-wide flock, never operator content",
	"internal/fabricengine/warpprobe.go": "probeWeftBinding's os.RemoveAll(probeDir) removes the throwaway probe clone directory the " +
		"same function created moments earlier",
	"internal/fabricengine/ancestors.go": "pruneEmptyAncestors' os.Remove(cur) is refused by the OS the moment the directory is " +
		"non-empty, and the loop halts on the first refusal",
	"internal/fabricengine/index.go": "refreshCorrIndexAfterSwitch's os.Remove(path) deliberately deletes the correspondence-index " +
		"cache before rebuilding it, so a failed refresh misses honestly rather than answering cross-branch",
	"internal/fabricengine/junction.go": "adoptDotLyxContent's os.Remove(link) removes the warp-side directory the adoption loop " +
		"immediately above it just emptied by rename — whole-file allowlist for this one audited site, not a blanket exemption",
	"internal/fabricengine/hook.go": "chainUserHook's os.Remove(userHookPath) removes the user-hook backup that same function wrote " +
		"ten lines earlier, on its own rollback path after a failed chain write",
	"internal/fabricengine/launchers.go": "removeLaunchers runs the gate's own checkPathRequest immediately before its os.Remove(launcherDir) " +
		"call, deliberately not removePath, since removePath's directory branch is RemoveAll, which would silently destroy foreign " +
		"content beside the launchers directory",
	"internal/fabricengine/doc.go": "the package doc's prose explains this slice's destruction rationale and must be able to name " +
		"the banned tokens; its only non-comment line is the package clause, so it can never carry a real call",
}

// destructiveGuardMinScannedFiles is the vacuous-scan floor for this guard's one-package walk:
// comfortably below the package's current production file count (53 at authoring time) and
// comfortably above zero. Its job is catching a misconfigured walk, not tracking the package's
// size — it is not expected to track file-count churn as the package grows or shrinks.
const destructiveGuardMinScannedFiles = 30

// TestNoDestructiveBypass_FabricengineProductionSource walks internal/fabricengine's non-test .go
// files and fails if any of them (other than a destructiveGuardAllowlist entry) contains one of
// destructiveGuardBannedTokens — the eight construction/call tokens a destructive primitive
// reached outside the gate would carry.
func TestNoDestructiveBypass_FabricengineProductionSource(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// rawgitmutation_test.go and tierpurity_test.go so this gate never blocks a minimal
	// environment.
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

	for _, pkgRel := range destructiveGuardScanPackages {
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
			// Normalize to slash-separated form before any comparison, exactly as
			// tierpurity_test.go/rawgitmutation_test.go do: filepath.WalkDir yields backslash
			// paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			scanned++

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(data)

			if _, allowlisted := destructiveGuardAllowlist[relPath]; allowlisted {
				return nil
			}

			for _, tok := range destructiveGuardBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains banned destructive-bypass token %q — a destructive primitive must be reached only through internal/fabricengine/destroy.go's gate (see CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant), or add a destructiveGuardAllowlist entry in cmd/lyx/destructiveguard_test.go with a reason if this is a new audited exemption",
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
	if scanned < destructiveGuardMinScannedFiles {
		t.Fatalf("destructive bypass guard: only scanned %d production .go file(s) across %v; expected at least %d — the walk may be misconfigured", scanned, destructiveGuardScanPackages, destructiveGuardMinScannedFiles)
	}

	if len(failures) > 0 {
		t.Errorf("Fabric Destruction Chokepoint Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}
