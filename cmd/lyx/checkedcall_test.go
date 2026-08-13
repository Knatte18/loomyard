// checkedcall_test.go enforces the gitexec Checked-Call Invariant: gitexec.Run/runChecked is the
// default, and every remaining raw gitexec.RunGit or r.run( call site in non-test source carries an
// adjacent //gitexec:raw marker justifying why the raw form is correct there.
// Test files are exempt from the marker requirement entirely — this guard never scans a _test.go
// file's content for either token, the same exemption every sibling guard in this package grants.
// See CONSTRAINTS.md's gitexec Checked-Call Invariant.
//
// # The two-token spelling contrast
//
// This guard's two raw tokens are spelled the OPPOSITE way from cmd/lyx/tierpurity_test.go's,
// cmd/lyx/hermeticenv_test.go's, and cmd/lyx/rawgitmutation_test.go's own gitexec tokens, and that
// contrast is deliberate, not an inconsistency to "fix":
//
//   - Those three guards ban ANY git spawn outright, so they key on the shorter prefix
//     "gitexec.Run" — its extra matches (catching gitexec.Run alongside gitexec.RunGit) are exactly
//     what a spawn-ban wants: neither entry point may appear in the files those guards police.
//   - This guard instead distinguishes the raw form from the checked one, so the same short prefix
//     would be wrong here: it would match every migrated gitexec.Run call site too and wrongly
//     demand a //gitexec:raw marker at each one. This guard therefore keys on the full,
//     unabbreviated "gitexec.RunGit" — without the trailing paren, so both the call form and a
//     doc-comment mention are still visible to a human reading a match, but is still specific enough
//     to exclude gitexec.Run.
//   - The same logic applies to internal/gitrepo's own pair: "r.run(" (WITH the trailing paren) is
//     this guard's second token, because the checked method's name, runChecked, contains "r.run" as
//     a substring. An unparenthesised "r.run" token would match every one of gitrepo's nineteen
//     migrated r.runChecked( call sites and demand a marker at each of them. "r.run(" — call syntax,
//     not just the bare method-name substring — matches only the raw method itself.
//
// A later reader comparing all four guard files side by side will be tempted to "harmonise" the
// spelling to match the other three. Doing so silently inverts this guard: it would stop flagging
// the raw sites this invariant exists to police and start flagging every migrated checked call
// instead.
//
// # Why the walk covers cmd/ too, even though zero production sites live there today
//
// Every one of the five pinned raw sites lives under internal/ (three in internal/gitrepo, two in
// internal/fabricengine); cmd/ carries none. The walk still covers cmd/ because that absence is
// exactly the fact the pinned-zero design is meant to keep true: a new command package reaching
// straight for gitexec.RunGit or a bespoke r.run( helper, bypassing gitrepo's own checked pair
// entirely, is precisely the drift this invariant exists to catch — and it can only catch it in a
// directory this guard actually visits.
//
// # The known blind spot
//
// This guard, like cmd/lyx/destructiveguard_test.go's and cmd/lyx/gitrepoboundary_test.go's own
// guards, is a raw-substring scan, not an AST walk: it cannot see a raw call written in a spelling
// its two literal tokens do not match (a reformatted call split across lines so "r.run(" or
// "gitexec.RunGit" never appears intact on one line, for instance), and it cannot tell a genuinely
// new raw call sitting a few lines inside an already-//gitexec:raw-marked region from the one call
// the marker was actually written to justify — the adjacency check only proves SOME raw-marked call
// is nearby, not that the flagged line is the one the marker's justification is actually about.
// Reviewing a diff that touches an already-marked region by hand remains necessary; this guard
// catches every other regression shape (a brand-new raw call with no marker at all, or one appearing
// in a package with no pinned allowance).

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// checkedCallRawTokens are the two raw substrings a non-test .go file under internal/ or cmd/ may
// contain only with an adjacent //gitexec:raw marker: gitexec.RunGit (the raw entry point, no
// trailing paren so a doc-comment mention still counts as a site needing acknowledgement) and
// r.run( (the raw half of internal/gitrepo's own checked/raw pair, WITH the trailing paren — see
// this file's header comment for why the parenthesised form is the only one that does not also
// match r.runChecked().
var checkedCallRawTokens = []string{
	"gitexec.RunGit",
	"r.run(",
}

// checkedCallPinnedRawSites is the literal, pinned per-package count of raw sites this invariant
// permits, keyed by module-relative, slash-separated package directory. A zero is listed explicitly
// rather than omitted — see this map's own package list for the ones the migration verified carry
// none. A package with no entry here at all is treated as pinned zero by this guard, not skipped:
// see checkedCallCountDiff.
var checkedCallPinnedRawSites = map[string]int{
	"internal/gitrepo":       3, // run's own body, Pull, Fetch
	"internal/fabricengine":  2, // weftRepoExists, weftBranchExists
	"internal/lyxcwd":        0,
	"internal/fabriccli":     0,
	"internal/websterengine": 0,
}

// checkedCallScanRoots are the module-relative directories this guard walks. cmd/ carries zero
// pinned raw sites today (see this file's header comment for why it is still walked).
var checkedCallScanRoots = []string{"internal", "cmd"}

// checkedCallMinScannedFiles is the vacuous-scan floor for this guard's two-root walk of internal/
// and cmd/: comfortably below the module's current non-test .go file count under those two roots
// (327 at authoring time) and comfortably above zero, so a misconfigured walk fails loudly rather
// than silently passing on a near-empty scan.
const checkedCallMinScannedFiles = 200

// TestCheckedCallInvariant_RawSitesMarkedAndPinned walks every non-test .go file under internal/ and
// cmd/ and asserts two things: (1) every line containing a checkedCallRawTokens substring carries an
// adjacent //gitexec:raw marker, on the same line or the line immediately above, and (2) the
// resulting per-package raw-site count matches checkedCallPinnedRawSites exactly, treating an unlisted
// package as pinned zero.
func TestCheckedCallInvariant_RawSitesMarkedAndPinned(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring every sibling
	// guard in this package so this gate never blocks a minimal environment.
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
	var markerFailures []string
	pkgRawCounts := map[string]int{}

	for _, rootRel := range checkedCallScanRoots {
		rootDir := filepath.Join(moduleRoot, rootRel)

		walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
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
			// Normalize to slash-separated form before any comparison: filepath.WalkDir yields
			// backslash paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			pkg := filepath.ToSlash(filepath.Dir(relPath))
			scanned++

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			lines := strings.Split(string(data), "\n")

			for i, line := range lines {
				// A pure-comment line (a doc comment mentioning either token in prose, such as a
				// package or file header) is not a call site and carries no marker requirement of
				// its own; only a line containing real source is a raw site.
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}

				matchedToken := ""
				for _, tok := range checkedCallRawTokens {
					if strings.Contains(line, tok) {
						matchedToken = tok
						break
					}
				}
				if matchedToken == "" {
					continue
				}

				markerHere := strings.Contains(line, "//gitexec:raw")
				markerAbove := i > 0 && strings.Contains(lines[i-1], "//gitexec:raw")
				if !markerHere && !markerAbove {
					markerFailures = append(markerFailures, fmt.Sprintf(
						"%s:%d: contains raw token %q with no adjacent //gitexec:raw marker (same line or the line immediately above) — mark it with the justification if the raw form is correct here, or migrate the call to the checked entry point",
						relPath, i+1, matchedToken,
					))
					continue
				}

				pkgRawCounts[pkg]++
			}

			return nil
		})
		if walkErr != nil {
			t.Fatalf("failed to walk %s: %v", rootDir, walkErr)
		}
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < checkedCallMinScannedFiles {
		t.Fatalf("checked-call guard: only scanned %d non-test .go file(s) under %v; expected at least %d — the walk may be misconfigured", scanned, checkedCallScanRoots, checkedCallMinScannedFiles)
	}

	sort.Strings(markerFailures)
	if len(markerFailures) > 0 {
		t.Errorf("gitexec Checked-Call Invariant violated (see CONSTRAINTS.md): unmarked raw call site(s):\n%s", strings.Join(markerFailures, "\n"))
	}

	if diff := checkedCallCountDiff(checkedCallPinnedRawSites, pkgRawCounts); diff != "" {
		t.Errorf("gitexec Checked-Call Invariant violated (see CONSTRAINTS.md): per-package raw-site count drifted from the pinned map:\n%s", diff)
	}
}

// checkedCallCountDiff compares got's per-package raw-site counts against want, treating any package
// present in got but absent from want as pinned zero. It returns a description of every mismatch, or
// "" if got matches want exactly, naming each drifted package, its actual raw-site count, and the
// two-line remedy: add the //gitexec:raw marker at the new site, then add or update the package's
// entry in checkedCallPinnedRawSites.
func checkedCallCountDiff(want, got map[string]int) string {
	packages := map[string]bool{}
	for pkg := range want {
		packages[pkg] = true
	}
	for pkg := range got {
		packages[pkg] = true
	}

	var names []string
	for pkg := range packages {
		names = append(names, pkg)
	}
	sort.Strings(names)

	var lines []string
	for _, pkg := range names {
		expected := want[pkg] // zero value when pkg has no map entry: an unlisted package is pinned zero.
		actual := got[pkg]
		if expected == actual {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"  %s: found %d raw site(s), pinned at %d — if this is a new, deliberate raw site, add its //gitexec:raw marker and add or update this package's entry in checkedCallPinnedRawSites",
			pkg, actual, expected,
		))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// TestCheckedCallInvariant_TokenSpellingsDoNotCollide pins the exact spelling contrast this guard's
// header comment documents: "r.run(" must not match a runChecked( call, and "gitexec.RunGit" must
// not match a gitexec.Run( call. This is the one place a plausible-looking edit — dropping the
// trailing paren from "r.run(", or shortening "gitexec.RunGit" to "gitexec.Run" to "harmonise" with
// the three token guards this file's header comment contrasts against — would silently invert this
// guard, so the contrast is asserted directly rather than left to be inferred from the header prose
// alone.
func TestCheckedCallInvariant_TokenSpellingsDoNotCollide(t *testing.T) {
	if strings.Contains("r.runChecked(msg)", "r.run(") {
		t.Error(`the "r.run(" token unexpectedly matches an r.runChecked( call — this would demand a //gitexec:raw marker at every migrated gitrepo call site`)
	}
	if strings.Contains("gitexec.Run(args, cwd)", "gitexec.RunGit") {
		t.Error(`the "gitexec.RunGit" token unexpectedly matches a gitexec.Run( call — this would demand a //gitexec:raw marker at every migrated call site`)
	}
}
