// overlay_seam_guard_test.go enforces the Fabric Git Invariant at the recipe-shape level.
// A BurlerFix row whose target paths enter the overlay tree (the durable _lym directory) must run fix-scope: overlay.
// Its partner Bouncer row must carry a commit_seam key matching the overlay subdirectory those target paths share.
// Otherwise an approved overlay-round fix has no committer,
// and the round's own writes stay uncommitted in the records working tree.
//
// The rule lives in one helper, assertOverlayBurlerCommitSeams.
// Both the embedded-recipe check and the synthetic cases call it.
// Sharing it keeps the two from drifting into separate copies of the rule.

package loomrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// assertOverlayBurlerCommitSeams walks every shedbuild.Row in r whose Engine is "BurlerFix".
// For each row that declares an overlay target path, it asserts the Fabric Git Invariant's overlay-seam rule.
// An overlay target path is one whose first segment equals lymdirs.LymDirName.
// The row's profile.fix-scope must be "overlay".
// The partner Bouncer row (same Segment) must carry a commit_seam config value.
// That value must equal the single overlay subdirectory the row's overlay target paths share.
//
// A row with no profile, no target, or no target paths is outside the rule and is skipped silently.
// That absence is the structural exemption, not an allowlist entry.
// A row whose target paths include none under the overlay tree is likewise skipped.
// An unexpected shape fails the test via t.Fatalf: paths that are not strings, or a profile or target that is not a map.
// A row skipped silently would leave the guard guarding nothing.
//
// Both halves report via t.Errorf rather than t.Fatalf.
// A single run against an unmodified recipe then surfaces both failures at once.
//
// t is typed testing.TB so a caller can pass a capturing implementation.
// That lets a rejection case observe the rule's t.Errorf calls without failing the enclosing test.
// A concrete *testing.T cannot do this: its Errorf failure propagates to every ancestor test,
// and the testing package offers no public API to suppress it.
func assertOverlayBurlerCommitSeams(t testing.TB, r shedbuild.Recipe) {
	t.Helper()

	for _, row := range r.Producers {
		if row.Engine != "BurlerFix" {
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
			if len(segments) < 2 || segments[0] != lymdirs.LymDirName {
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

// assertFixScopeOverlay checks that row's profile.fix-scope key is present and equal to "overlay".
// A missing key and a non-overlay value each go to their own t.Errorf call, naming row.
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

// assertCommitSeam checks the seam half of the overlay-seam rule for burlerRow.
// The overlay target paths of burlerRow must share one overlay subdirectory.
// Its partner Bouncer row (same Segment within r) must carry a commit_seam config equal to that subdirectory.
// Each failure goes to t.Errorf and names the offending row.
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

// TestShippedRecipe_OverlayBurlersCommitThroughSeam parses the embedded recipes.TapestryRecipe.
// It runs assertOverlayBurlerCommitSeams on the result, a read of the shipped file.
//
//testtiming:keep pins that the shipped overlay Burlers commit through their partner Bouncer's seam, which TestOverlayBurlerCommitSeams_RejectsBadShapes only checks on hand-written recipes
func TestShippedRecipe_OverlayBurlersCommitThroughSeam(t *testing.T) {
	tiergate.Require(t, 0)
	r, err := shedbuild.Parse(recipes.TapestryRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.TapestryRecipe) error = %v; want nil", err)
	}
	assertOverlayBurlerCommitSeams(t, r)
}

// capturingT is a testing.TB that records failures instead of reporting them.
// It lets a case of TestOverlayBurlerCommitSeams_RejectsBadShapes check for a rejection without failing the outer test.
type capturingT struct {
	testing.TB
	failed bool
}

// Errorf records a reported failure without propagating it to the outer test.
func (c *capturingT) Errorf(format string, args ...any) {
	c.failed = true
}

// Fatalf records the failure the same way Errorf does, and returns instead of halting the goroutine.
func (c *capturingT) Fatalf(format string, args ...any) {
	c.failed = true
}

// Helper does nothing, since capturingT tracks no call stack.
func (c *capturingT) Helper() {}

// runOverlaySeamRule runs assertOverlayBurlerCommitSeams on r in a subtest named name.
// It reports whether the rule reported any failure, captured through a capturingT wrapping t.
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

// TestOverlayBurlerCommitSeams_RejectsBadShapes checks the rule against small hand-written recipe documents.
// Each document is parsed by shedbuild.Parse.
// The rule must report a failure for each rejected shape and none for each accepted one.
//
//testtiming:keep pins that the seam rule rejects each bad overlay shape, a guard self-check the shipped-recipe test never exercises
func TestOverlayBurlerCommitSeams_RejectsBadShapes(t *testing.T) {
	tiergate.Require(t, 0)
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

// overlaySeamFixture returns a two-row recipe YAML document: a Bouncer and a BurlerFix row in segment "Fixture-Review".
// The Burler row carries fixScope as its profile.fix-scope value.
// The Bouncer row carries commitSeam as its commit_seam config value, and omits the key when commitSeam is empty.
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
		"    engine: BurlerFix\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lym/discussion/decision-record.md\n" +
		"        fix-scope: " + fixScope + "\n"
}

// overlaySeamFixtureNoBouncer returns a one-row recipe YAML document:
// an overlay-targeting Burler row whose segment names no Bouncer row in the document.
func overlaySeamFixtureNoBouncer() string {
	return "version: 1\n" +
		"entry: Fixture-Burler\n" +
		"terminals: [Fixture-Burler]\n" +
		"producers:\n" +
		"  - name: Fixture-Burler\n" +
		"    engine: BurlerFix\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lym/discussion/decision-record.md\n" +
		"        fix-scope: overlay\n"
}

// overlaySeamFixtureStraddling returns a two-row recipe YAML document.
// Its Burler row's target paths span two overlay subdirectories, "discussion" and "plan".
// Its Bouncer row sets commit_seam to discussion.
// The straddle is the rejected shape, checked ahead of the seam-value check.
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
		"    engine: BurlerFix\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          paths:\n" +
		"            - _lym/discussion/decision-record.md\n" +
		"            - _lym/plan/00-overview.md\n" +
		"        fix-scope: overlay\n"
}

// overlaySeamFixtureInstructionsOnly returns a two-row recipe YAML document.
// Its Burler row declares profile.target.instructions and no paths key, and runs fix-scope: source.
// The rule must pass this shape as a structural exemption, without inspecting fix-scope or commit_seam.
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
		"    engine: BurlerFix\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      profile:\n" +
		"        target:\n" +
		"          instructions: read the diff\n" +
		"        fix-scope: source\n"
}
