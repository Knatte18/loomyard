// verifiedtree_test.go enforces `PATTERN-verified-tree`:
// every plan-verify site goes through verifytree.Verify, so none verifies a dirty tree and none skips without the record.
// It is a tripwire over production source, like constraintchokepoint_test.go: a same-line substring scan that skips comment-only lines.
// It catches a site that bypasses verifytree.Verify or brings back the retired verify-pending marker, not every way a verify could be spelled.

package main

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// planVerifySites are the production files that run the plan's verify command, each of which must call verifytree.Verify.
var planVerifySites = []string{
	"internal/websterengine/verifygate.go",
	"internal/loomshed/verifygate.go",
	"internal/webstercli/verify.go",
	"internal/landingshed/verifygate.go",
}

// verifyRunAllowed names the files outside internal/verifytree that may call verifyrun.Run, each with its reason.
var verifyRunAllowed = map[string]string{
	"internal/websterengine/cardverify.go": "reruns a card's own **Verify:** command, which is not a plan-verify site",
}

const retiredMarkerName = "verify-pending"

// codeLines returns data's lines that are not comment-only.
func codeLines(data []byte) []string {
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// callsAny reports whether any of data's code lines contains token.
func callsAny(data []byte, token string) bool {
	for _, l := range codeLines(data) {
		if strings.Contains(l, token) {
			return true
		}
	}
	return false
}

// TestVerifiedTree_PlanVerifySitesCallVerify fails when a plan-verify site does not call or hold verifytree.Verify.
func TestVerifiedTree_PlanVerifySitesCallVerify(t *testing.T) {
	seen := map[string]bool{}
	scankit.Walk(t, scankit.Options{Roots: []string{"internal"}}, func(f *scankit.File) {
		for _, site := range planVerifySites {
			if f.Rel == site {
				seen[site] = true
				// Without the paren: the landing gate holds the function as a seam value, not a direct call.
				if !callsAny(f.Data, "verifytree.Verify") {
					t.Errorf("`PATTERN-verified-tree` violated: %s is a plan-verify site and does not call verifytree.Verify", site)
				}
			}
		}
	})
	for _, site := range planVerifySites {
		if !seen[site] {
			t.Errorf("plan-verify site %s was not found; update planVerifySites if it moved", site)
		}
	}
}

// TestVerifiedTree_NoOtherVerifyRunCaller fails when a production file outside internal/verifytree calls verifyrun.Run.
func TestVerifiedTree_NoOtherVerifyRunCaller(t *testing.T) {
	allowedSeen := map[string]bool{}
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/verifytree/") || !callsAny(f.Data, "verifyrun.Run(") {
			return
		}
		if _, ok := verifyRunAllowed[f.Rel]; ok {
			allowedSeen[f.Rel] = true
			return
		}
		t.Errorf("`PATTERN-verified-tree` violated: %s calls verifyrun.Run; a plan verify goes through verifytree.Verify", f.Rel)
	})
	scankit.RequireFloor(t, scanned, 20, "verified-tree verifyrun scan")
	for rel := range verifyRunAllowed {
		if !allowedSeen[rel] {
			t.Errorf("verifyRunAllowed names %s, which no longer calls verifyrun.Run; drop the entry", rel)
		}
	}
}

// TestVerifiedTree_NoRetiredMarker fails when a production file names the retired verify-pending marker.
func TestVerifiedTree_NoRetiredMarker(t *testing.T) {
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
		if callsAny(f.Data, retiredMarkerName) {
			t.Errorf("`PATTERN-verified-tree` violated: %s names the retired %s marker", f.Rel, retiredMarkerName)
		}
	})
	scankit.RequireFloor(t, scanned, 20, "verified-tree retired-marker scan")
}

// TestVerifiedTree_ScanHelpersFireOnOffendingStayQuietOnCompliant proves the scan helpers fire on an offending source and stay quiet on a compliant or commented one.
func TestVerifiedTree_ScanHelpersFireOnOffendingStayQuietOnCompliant(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		token string
		want  bool
	}{
		{name: "direct call", src: "code, err := verifyrun.Run(ctx, cmd, dir, w)\n", token: "verifyrun.Run(", want: true},
		{name: "comment-only mention", src: "// calls verifyrun.Run( for a card\n", token: "verifyrun.Run(", want: false},
		{name: "retired marker in code", src: "name := \"verify-pending\"\n", token: retiredMarkerName, want: true},
		{name: "retired marker in a comment", src: "\t// the retired verify-pending marker\n", token: retiredMarkerName, want: false},
	}
	for _, tt := range tests {
		if got := callsAny([]byte(tt.src), tt.token); got != tt.want {
			t.Errorf("%s: callsAny = %v; want %v", tt.name, got, tt.want)
		}
	}
}
