// overlay_seam_guard_test.go enforces the Fabric Git Invariant at the recipe-shape level: every
// BurlerRound row whose target paths reach into the overlay tree (the durable _lyx directory) must
// run fix-scope: overlay, and its partner Bouncer row must carry a commit_seam key matching the
// overlay subdirectory those target paths share -- otherwise an approved overlay-round fix has no
// committer, and the round's own writes stay uncommitted in the weft working tree.
//
// This is the parse-level regression guard the shipped Discussion-Burler row's fix-scope: source
// violation would have caught before it shipped: assertOverlayBurlerCommitSeams is one rule shared
// by TestShippedRecipe_OverlayBurlersCommitThroughSeam (the real embedded recipe) and
// TestOverlayBurlerCommitSeams_RejectsBadShapes (synthetic negative and positive cases), so the two
// halves can never drift into two hand-kept copies of the same rule.

package loomrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// assertOverlayBurlerCommitSeams walks every shedbuild.Row in r whose Engine is "BurlerRound" and,
// for each row that declares at least one overlay target path (a path whose first segment equals
// lyxdirs.LyxDirName), asserts both halves of the Fabric Git Invariant's overlay-seam rule: the
// row's profile.fix-scope must be "overlay", and the row's partner Bouncer row (same Segment) must
// carry a commit_seam config value equal to the single overlay subdirectory the row's overlay target
// paths share.
//
// A row declaring no profile, no target, or no target paths at all is outside the rule and is
// skipped silently -- that absence is the structural exemption, not an allowlist entry. A row whose
// target paths include none under the overlay tree is likewise skipped. An unexpected shape (paths
// present but non-string, or profile/target present but not a map) is reported via t.Fatalf, naming
// the row, because a silently-skipped row here is a guard that does not guard.
//
// Both rule-violation halves are reported via t.Errorf rather than t.Fatalf so a single run against
// an unmodified recipe can surface both failures at once, which is the proof this guard's own
// TDD step requires.
//
// t is typed testing.TB rather than the concrete *testing.T: TestOverlayBurlerCommitSeams_RejectsBadShapes
// hands this function a capturingT so a case asserting rejection can observe the rule's own t.Errorf
// calls without failing the outer test, which a concrete *testing.T's real Errorf makes impossible --
// a subtest's real Errorf failure always propagates to fail every ancestor test, with no public API
// to suppress it.
func assertOverlayBurlerCommitSeams(t testing.TB, r shedbuild.Recipe) {
	t.Helper()

	for _, row := range r.Producers {
		if row.Engine != "BurlerRound" {
			continue
		}

		profileRaw, ok := row.Config["profile"]
		if !ok {
			continue
		}
		profile, ok := profileRaw.(map[string]any)
		if !ok {
			t.Fatalf("row %q: config[\"profile\"] is %T; want map[string]any", row.Name, profileRaw)
		}

		targetRaw, ok := profile["target"]
		if !ok {
			continue
		}
		target, ok := targetRaw.(map[string]any)
		if !ok {
			t.Fatalf("row %q: profile[\"target\"] is %T; want map[string]any", row.Name, targetRaw)
		}

		pathsRaw, ok := target["paths"]
		if !ok {
			continue
		}
		pathsSlice, ok := pathsRaw.([]any)
		if !ok {
			t.Fatalf("row %q: profile.target[\"paths\"] is %T; want []any", row.Name, pathsRaw)
		}

		paths := make([]string, len(pathsSlice))
		for i, p := range pathsSlice {
			s, ok := p.(string)
			if !ok {
				t.Fatalf("row %q: profile.target.paths[%d] is %T; want string", row.Name, i, p)
			}
			paths[i] = s
		}

		var overlaySubdirs []string
		var isOverlay bool
		for _, p := range paths {
			segments := strings.SplitN(p, "/", 3)
			if len(segments) < 2 || segments[0] != lyxdirs.LyxDirName {
				continue
			}
			isOverlay = true
			overlaySubdirs = append(overlaySubdirs, segments[1])
		}
		if !isOverlay {
			continue
		}

		assertFixScopeOverlay(t, row)
		assertCommitSeam(t, r, row, overlaySubdirs)
	}
}

// assertFixScopeOverlay reports whether row's profile.fix-scope key is present and equal to
// "overlay", via two distinct t.Errorf calls naming row -- one for a missing key, one for a
// non-overlay value -- so a single guard run can surface both this half's failure and
// assertCommitSeam's failure together.
func assertFixScopeOverlay(t testing.TB, row shedbuild.Row) {
	t.Helper()

	profile := row.Config["profile"].(map[string]any)
	fixScopeRaw, ok := profile["fix-scope"]
	if !ok {
		t.Errorf("row %q: declares overlay target paths but profile has no \"fix-scope\" key; want \"overlay\"", row.Name)
		return
	}
	if fixScopeRaw != "overlay" {
		t.Errorf("row %q: profile[\"fix-scope\"] = %v; want \"overlay\" (row declares overlay target paths)", row.Name, fixScopeRaw)
	}
}

// assertCommitSeam reports whether burlerRow's overlay target paths share a single overlay
// subdirectory and, if so, whether burlerRow's partner Bouncer row (same Segment within r) carries a
// commit_seam config value equal to that subdirectory. Every failure is reported via t.Errorf naming
// the offending row, for the same both-halves-in-one-run reason assertFixScopeOverlay documents.
func assertCommitSeam(t testing.TB, r shedbuild.Recipe, burlerRow shedbuild.Row, overlaySubdirs []string) {
	t.Helper()

	subdir := overlaySubdirs[0]
	for _, s := range overlaySubdirs[1:] {
		if s != subdir {
			t.Errorf("row %q: overlay target paths straddle two overlay subdirectories (%q and %q); a row must target exactly one", burlerRow.Name, subdir, s)
			return
		}
	}

	var bouncerRow *shedbuild.Row
	for i := range r.Producers {
		if r.Producers[i].Engine == "Bouncer" && r.Producers[i].Segment == burlerRow.Segment {
			bouncerRow = &r.Producers[i]
			break
		}
	}
	if bouncerRow == nil {
		t.Errorf("row %q: segment %q has no partner Bouncer row", burlerRow.Name, burlerRow.Segment)
		return
	}

	commitSeamRaw, ok := bouncerRow.Config["commit_seam"]
	if !ok {
		t.Errorf("row %q: partner Bouncer row %q has no \"commit_seam\" key; want %q", burlerRow.Name, bouncerRow.Name, subdir)
		return
	}
	commitSeam, ok := commitSeamRaw.(string)
	if !ok || commitSeam == "" {
		t.Errorf("row %q: partner Bouncer row %q has no \"commit_seam\" key; want %q", burlerRow.Name, bouncerRow.Name, subdir)
		return
	}
	if commitSeam != subdir {
		t.Errorf("row %q: partner Bouncer row %q commit_seam = %q; want %q", burlerRow.Name, bouncerRow.Name, commitSeam, subdir)
	}
}

// TestShippedRecipe_OverlayBurlersCommitThroughSeam parses the real embedded recipes.LoomRecipe and
// runs assertOverlayBurlerCommitSeams against it, unmodified -- a straight read of the shipped file,
// never a mutated copy.
func TestShippedRecipe_OverlayBurlersCommitThroughSeam(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}
	assertOverlayBurlerCommitSeams(t, r)
}

// capturingT is a minimal *testing.T-shaped recorder assertOverlayBurlerCommitSeams's t.Errorf calls
// write into, letting TestOverlayBurlerCommitSeams_RejectsBadShapes assert a case reported a failure
// without failing the outer test itself.
type capturingT struct {
	testing.TB
	failed bool
}

// Errorf implements the subset of testing.TB assertOverlayBurlerCommitSeams calls, recording that a
// failure was reported rather than propagating it to the outer test.
func (c *capturingT) Errorf(format string, args ...any) {
	c.failed = true
}

// Fatalf implements the subset of testing.TB assertOverlayBurlerCommitSeams calls. It records the
// failure the same way Errorf does and returns rather than halting the goroutine: every synthetic
// case below is constructed so the rule never reaches a Fatalf-shaped condition on a case that must
// continue past it, so a Fatalf that returns still leaves c.failed true, which is all this helper's
// callers observe.
func (c *capturingT) Fatalf(format string, args ...any) {
	c.failed = true
}

// Helper implements the subset of testing.TB assertOverlayBurlerCommitSeams calls; it is a no-op
// here since capturingT tracks no call stack.
func (c *capturingT) Helper() {}

// runOverlaySeamRule runs assertOverlayBurlerCommitSeams against r using a capturingT wrapping t,
// via a nested t.Run so the case's own name appears in test output, and returns whether the rule
// reported any failure.
func runOverlaySeamRule(t *testing.T, name string, r shedbuild.Recipe) (reported bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		c := &capturingT{TB: t}
		assertOverlayBurlerCommitSeams(c, r)
		reported = c.failed
	})
	return reported
}

// TestOverlayBurlerCommitSeams_RejectsBadShapes table-drives assertOverlayBurlerCommitSeams over
// small hand-written recipe YAML documents, each fed through the identical shedbuild.Parse the
// shipped-recipe test uses, asserting the rule reports (or, for the two accept cases, does not
// report) a failure for the shape each case names.
func TestOverlayBurlerCommitSeams_RejectsBadShapes(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantReject bool
	}{
		{
			name:       "overlay-targeting Burler carries fix-scope: source",
			yaml:       overlaySeamFixture("source", "discussion"),
			wantReject: true,
		},
		{
			name:       "fix-scope: overlay but partner Bouncer omits commit_seam entirely",
			yaml:       overlaySeamFixture("overlay", ""),
			wantReject: true,
		},
		{
			name:       "partner Bouncer carries the wrong seam value",
			yaml:       overlaySeamFixture("overlay", "wrong-subdir"),
			wantReject: true,
		},
		{
			name:       "segment names no Bouncer row at all",
			yaml:       overlaySeamFixtureNoBouncer(),
			wantReject: true,
		},
		{
			name:       "target paths straddle two overlay subdirectories",
			yaml:       overlaySeamFixtureStraddling(),
			wantReject: true,
		},
		{
			name:       "Burler declaring only instructions and no target paths passes with fix-scope: source",
			yaml:       overlaySeamFixtureInstructionsOnly(),
			wantReject: false,
		},
		{
			name:       "well-formed overlay Burler with correctly-valued partner seam passes",
			yaml:       overlaySeamFixture("overlay", "discussion"),
			wantReject: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := shedbuild.Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
			}
			reported := runOverlaySeamRule(t, tt.name, r)
			if reported != tt.wantReject {
				t.Errorf("assertOverlayBurlerCommitSeams reported = %v; want %v", reported, tt.wantReject)
			}
		})
	}
}

// overlaySeamFixture returns a two-row recipe YAML document -- a Bouncer/BurlerRound pair sharing
// segment "Fixture-Review" -- whose Burler row carries fixScope as its profile.fix-scope value and
// whose Bouncer row carries commitSeam as its commit_seam config value (omitted entirely when
// commitSeam is empty).
func overlaySeamFixture(fixScope, commitSeam string) string {
	commitSeamLine := ""
	if commitSeam != "" {
		commitSeamLine = "      commit_seam: " + commitSeam + "\n"
	}
	return "version: 1\n" +
		"entry: Fixture-Bouncer\n" +
		"terminals: [Fixture-Bouncer]\n" +
		"producers:\n" +
		"  - name: Fixture-Bouncer\n" +
		"    engine: Bouncer\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		commitSeamLine +
		"  - name: Fixture-Burler\n" +
		"    engine: BurlerRound\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lyx/discussion/decision-record.md\n" +
		"        fix-scope: " + fixScope + "\n"
}

// overlaySeamFixtureNoBouncer returns a one-row recipe YAML document: an overlay-targeting Burler
// row whose Segment names no Bouncer row anywhere in the document.
func overlaySeamFixtureNoBouncer() string {
	return "version: 1\n" +
		"entry: Fixture-Burler\n" +
		"terminals: [Fixture-Burler]\n" +
		"producers:\n" +
		"  - name: Fixture-Burler\n" +
		"    engine: BurlerRound\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lyx/discussion/decision-record.md\n" +
		"        fix-scope: overlay\n"
}

// overlaySeamFixtureStraddling returns a two-row recipe YAML document whose Burler row's target
// paths span two different overlay subdirectories ("discussion" and "plan"), with a correctly
// seamed partner Bouncer row for neither -- the straddle itself is the rejected shape, checked ahead
// of the seam-value check.
func overlaySeamFixtureStraddling() string {
	return "version: 1\n" +
		"entry: Fixture-Bouncer\n" +
		"terminals: [Fixture-Bouncer]\n" +
		"producers:\n" +
		"  - name: Fixture-Bouncer\n" +
		"    engine: Bouncer\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      commit_seam: discussion\n" +
		"  - name: Fixture-Burler\n" +
		"    engine: BurlerRound\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lyx/discussion/decision-record.md\n" +
		"            - _lyx/plan/00-overview.md\n" +
		"        fix-scope: overlay\n"
}

// overlaySeamFixtureInstructionsOnly returns a two-row recipe YAML document whose Burler row
// declares profile.target.instructions and no paths key at all, running fix-scope: source -- the
// structural exemption the rule must pass without ever inspecting fix-scope or commit_seam.
func overlaySeamFixtureInstructionsOnly() string {
	return "version: 1\n" +
		"entry: Fixture-Bouncer\n" +
		"terminals: [Fixture-Bouncer]\n" +
		"producers:\n" +
		"  - name: Fixture-Bouncer\n" +
		"    engine: Bouncer\n" +
		"    segment: Fixture-Review\n" +
		"    config: {}\n" +
		"  - name: Fixture-Burler\n" +
		"    engine: BurlerRound\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          instructions: read the diff\n" +
		"        fix-scope: source\n"
}
