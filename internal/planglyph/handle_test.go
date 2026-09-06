// handle_test.go covers CanonicalizeHandles: the batched Name call, positional matching, the
// Rename-derived source, failure and collision handling, and the on-disk rewrite.

package planglyph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// writePlanFixture writes a minimal, valid on-disk plan directory from cards (keyed by card
// number, valued by the card body markdown following the "# Card N — cardN" heading), then parses
// it, returning both the directory and the parsed *planparser.Plan.
func writePlanFixture(t *testing.T, cards map[int]string) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()

	numbers := make([]int, 0, len(cards))
	for n := range cards {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	var indexLines []string
	for _, n := range numbers {
		slug := fmt.Sprintf("card%d", n)
		indexLines = append(indexLines, fmt.Sprintf("%d — %s — summary", n, slug))
		content := fmt.Sprintf("# Card %d — %s\n\n%s\n", n, slug, cards[n])
		path := filepath.Join(dir, fmt.Sprintf("%02d-%s.md", n, slug))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", path, err)
		}
	}

	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan: test\n\nframing\n\n## Card Index\n\n" +
		strings.Join(indexLines, "\n") + "\n"
	overviewPath := filepath.Join(dir, "00-overview.md")
	if err := os.WriteFile(overviewPath, []byte(overview), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", overviewPath, err)
	}

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	return dir, plan
}

// readCardFile returns card n's own file content from dir.
func readCardFile(t *testing.T, dir string, n int, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%02d-%s.md", n, slug)))
	if err != nil {
		t.Fatalf("ReadFile(card %d) failed: %v", n, err)
	}
	return string(data)
}

func TestCanonicalizeHandles_BatchedCallCoversBothSources(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Old() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Old"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#A` -> `func A() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#B` -> `func B() {}`\n\n**Intent:** two\n",
		3: "**Create:**\n- `plan:sub#C` -> `func C() {}`\n\n**Intent:** three\n",
		4: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** four\n\n## Rename mechanic\n",
	})
	// plan.Dir must equal dir for CanonicalizeHandles's caller shape; ParsePlan already stamps it.
	if plan.Dir != dir {
		t.Fatalf("plan.Dir = %q; want %q", plan.Dir, dir)
	}

	findings, err := CanonicalizeHandles(plan, dir, results)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	for _, f := range findings {
		t.Errorf("unexpected finding: %+v", f)
	}

	// Each Create declaration's own draft handle already names its computed identifier verbatim
	// (plan:sub#A -> func A() {}), so canonicalization is a no-op rewrite for those three; the
	// point of this test is that all four sources -- three Create declarations plus one
	// Rename-derived declaration -- rode the same batched call and none reported a finding.
	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	got3 := readCardFile(t, dir, 3, "card3")
	got4 := readCardFile(t, dir, 4, "card4")
	for i, got := range []string{got1, got2, got3} {
		if !strings.Contains(got, "plan:sub#") {
			t.Errorf("card %d lost its handle prefix entirely: %s", i+1, got)
		}
	}
	if !strings.Contains(got4, "plan:sub#") {
		t.Errorf("card 4 lost its handle prefix entirely: %s", got4)
	}
}

func TestCanonicalizeHandles_PositionalMatchingOutOfOrder(t *testing.T) {
	// Card 1's handle sorts after card 2's ("Z" > "A"), and each declared identifier deliberately
	// differs from its own handle's member name: a positional mismatch in matching Name's results
	// back to their sources would cross-wire these two and produce a detectably wrong rewrite.
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#Z` -> `func ActualZ() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#A` -> `func ActualA() {}`\n\n**Intent:** two\n",
	})

	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if !strings.Contains(got1, "plan:sub#ActualZ") {
		t.Errorf("card 1's own handle did not canonicalize to plan:sub#ActualZ: %s", got1)
	}
	if !strings.Contains(got2, "plan:sub#ActualA") {
		t.Errorf("card 2's own handle did not canonicalize to plan:sub#ActualA: %s", got2)
	}
}

func TestCanonicalizeHandles_RenameDraftSpellingWrongStillRewrites(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Old() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Old"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	// The draft to-side deliberately misspells the unit half ("wrongpkg" instead of "sub", where
	// the old side actually resolved): canonicalization derives the Unit from Old's own resolved
	// glyph, never from the draft, so the wrong unit must not survive the rewrite.
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:wrongpkg#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	findings, err := CanonicalizeHandles(plan, dir, results)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got := readCardFile(t, dir, 1, "card1")
	if strings.Contains(got, "plan:wrongpkg#New") {
		t.Errorf("draft spelling %q survived canonicalization: %s", "plan:wrongpkg#New", got)
	}
	if !strings.Contains(got, "plan:sub#New") {
		t.Errorf("canonical handle plan:sub#New missing after rewrite: %s", got)
	}
}

func TestCanonicalizeHandles_RenameOldUnresolved(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#DoesNotExist` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	// No result at all for "sub#DoesNotExist": the same as it never resolving found.
	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "rename-old-unresolved" {
		t.Fatalf("findings = %+v; want exactly one rename-old-unresolved finding", findings)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#New") {
		t.Errorf("card was rewritten despite an unresolved old side: %s", got)
	}
}

func TestCanonicalizeHandles_OneFailingDeclarationLeavesOthersRewritten(t *testing.T) {
	// Card 1's draft handle deliberately misspells the declared identifier ("Good" versus the
	// declaration head's own "ActualGood") so its successful rewrite is verifiable against a
	// changed string, not merely a no-op round trip.
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#Good` -> `func ActualGood() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#Bad` -> `this is not valid go at all {{{`\n\n**Intent:** two\n",
	})

	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}

	failCount := 0
	for _, f := range findings {
		if f.Check == "handle-name-failed" {
			failCount++
		}
	}
	if failCount != 1 {
		t.Fatalf("findings = %+v; want exactly one handle-name-failed", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if strings.Contains(got1, "plan:sub#Good") {
		t.Errorf("card 1's good declaration was not rewritten: %s", got1)
	}
	if !strings.Contains(got2, "plan:sub#Bad") {
		t.Errorf("card 2's failing declaration was rewritten despite failing naming: %s", got2)
	}
}

func TestCanonicalizeHandles_CanonicalCollisionRewritesNeither(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#One` -> `func Same() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#Two` -> `func Same() {}`\n\n**Intent:** two\n",
	})

	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}

	collided := false
	for _, f := range findings {
		if f.Check == "handle-canonical-collision" {
			collided = true
		}
	}
	if !collided {
		t.Fatalf("findings = %+v; want a handle-canonical-collision finding", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if !strings.Contains(got1, "plan:sub#One") || !strings.Contains(got2, "plan:sub#Two") {
		t.Errorf("a colliding handle was rewritten despite the collision:\n%s\n%s", got1, got2)
	}
}

func TestCanonicalizeHandles_RewriteLandsOnEveryReferencingCard(t *testing.T) {
	// The declared identifier ("ActualNew") deliberately differs from the draft handle's own
	// member name ("New"), so the resulting rewrite is verifiable against a changed string on
	// both the declaring card and the referencing card.
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#New` -> `func ActualNew() {}`\n\n**Intent:** one\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})

	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if strings.Contains(got1, "plan:sub#New") {
		t.Errorf("declaring card 1 still carries the draft spelling: %s", got1)
	}
	if strings.Contains(got2, "plan:sub#New") {
		t.Errorf("referencing card 2 still carries the draft spelling: %s", got2)
	}
}

func TestCanonicalizeHandles_LanguageNoneNoOp(t *testing.T) {
	dir := t.TempDir()
	plan := &planparser.Plan{Dir: dir, Language: "none"}

	findings, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if findings != nil {
		t.Errorf("findings = %+v; want nil under language: none", findings)
	}
}
