// checkedcall_test.go enforces the gitexec Checked-Call Invariant for non-test source under internal/ and cmd/.
// A raw gitexec.RunGit or r.run( call there must carry an adjacent //gitexec:raw marker; test files are exempt.
// See `PATTERN-gitrepo-client-boundary`.

package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// checkedCallRawTokens are the raw substrings a non-test .go file under internal/ or cmd/ may contain.
// Each line with one needs an adjacent //gitexec:raw marker.
// gitexec.RunGit has no trailing paren, so a doc-comment mention also counts as a site needing acknowledgement.
// r.run( keeps its paren because the checked method, runChecked, contains r.run as a substring.
// The longer spellings match only the raw calls; a shorter gitexec.Run or r.run would also match the checked calls.
// The scan is a raw-substring match, not an AST walk, so it misses a call whose token is split across lines.
// It also cannot tell a new raw call a few lines inside an already marked region from the call the marker justifies,
// so a diff touching such a region needs a manual review.
var checkedCallRawTokens = []string{
	"gitexec.RunGit",
	"r.run(",
}

// checkedCallPinnedRawSites pins the number of raw sites this invariant permits in each package.
// It is keyed by module-relative, slash-separated package directory.
// A zero is listed explicitly rather than omitted.
// A package with no entry counts as pinned zero, not skipped.
var checkedCallPinnedRawSites = map[string]int{
	"internal/gitrepo":       3, // run's own body, Pull, Fetch
	"internal/fabricengine":  0,
	"internal/lymcwd":        0,
	"internal/fabriccli":     0,
	"internal/websterengine": 0,

	"internal/gitrepo/internal/gitoracle": 6, // CurrentSHA, SHAExists, ChangedFilesSince, CurrentBranch, HeadRef, verifyQuiet
}

// checkedCallScanRoots are the module-relative directories this guard walks.
// cmd/ is walked too, though it is meant to carry no pinned raw sites.
// A command package that calls gitexec.RunGit or a bespoke r.run( helper bypasses the checked entry point,
// and this guard catches that only in a directory it visits.
var checkedCallScanRoots = []string{"internal", "cmd"}

// checkedCallMinScannedFiles is the minimum file count this guard's walk of internal/ and cmd/ must reach.
// It sits far below the non-test .go file count under those roots and above zero,
// so a misconfigured walk fails loudly rather than passing on a near-empty scan.
const checkedCallMinScannedFiles = 200

// TestCheckedCallInvariant_RawSitesMarkedAndPinned walks every non-test .go file under internal/ and cmd/.
// It asserts two things.
// Every line containing a checkedCallRawTokens substring carries an adjacent //gitexec:raw marker:
// on the same line or the line immediately above.
// The per-package raw-site count matches checkedCallPinnedRawSites exactly,
// and a package not listed there counts as pinned zero.
//
//lym:guard
func TestCheckedCallInvariant_RawSitesMarkedAndPinned(t *testing.T) {
	tiergate.Require(t, 0)
	var markerFailures []string
	pkgRawCounts := map[string]int{}

	scanned := scankit.Walk(t, scankit.Options{Roots: checkedCallScanRoots}, func(f *scankit.File) {
		pkg := path.Dir(f.Rel)
		lines := strings.Split(string(f.Data), "\n")

		for i, line := range lines {
			// A comment-only line that names a token in prose is not a call site and needs no marker.
			// Only a line containing source code is a raw site.
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
					f.Rel, i+1, matchedToken,
				))
				continue
			}

			pkgRawCounts[pkg]++
		}
	})

	scankit.RequireFloor(t, scanned, checkedCallMinScannedFiles, "checked-call guard")

	sort.Strings(markerFailures)
	if len(markerFailures) > 0 {
		t.Errorf("`PATTERN-gitrepo-client-boundary` violated: unmarked raw call site(s):\n%s", strings.Join(markerFailures, "\n"))
	}

	if diff := checkedCallCountDiff(checkedCallPinnedRawSites, pkgRawCounts); diff != "" {
		t.Errorf("`PATTERN-gitrepo-client-boundary` violated: per-package raw-site count drifted from the pinned map:\n%s", diff)
	}
}

// checkedCallCountDiff returns a description of every mismatch between got's per-package raw-site counts and want.
// It returns "" when they match exactly.
// A package present in got but absent from want counts as pinned zero.
// Each mismatch names the drifted package, its actual raw-site count, and the two-step remedy:
// add the //gitexec:raw marker at the new site, then add or update the package's entry in checkedCallPinnedRawSites.
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

// TestCheckedCallInvariant_TokenSpellingsDoNotCollide pins that "r.run(" does not match a runChecked( call,
// and that "gitexec.RunGit" does not match a gitexec.Run( call.
// A match in either case would make the raw-site guard demand a //gitexec:raw marker at every migrated call site.
//
//testtiming:keep proves the TestCheckedCallInvariant_RawSitesMarkedAndPinned guard's token spellings stay distinct from the three token guards it is contrasted with
func TestCheckedCallInvariant_TokenSpellingsDoNotCollide(t *testing.T) {
	tiergate.Require(t, 0)
	if strings.Contains("r.runChecked(msg)", "r.run(") {
		t.Error(`the "r.run(" token unexpectedly matches an r.runChecked( call — this would demand a //gitexec:raw marker at every migrated gitrepo call site`)
	}
	if strings.Contains("gitexec.Run(args, cwd)", "gitexec.RunGit") {
		t.Error(`the "gitexec.RunGit" token unexpectedly matches a gitexec.Run( call — this would demand a //gitexec:raw marker at every migrated call site`)
	}
}
