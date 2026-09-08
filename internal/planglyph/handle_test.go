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
	"github.com/Knatte18/quarry/quarry"
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

	findings, _, err := CanonicalizeHandles(plan, dir, results)
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

	findings, _, err := CanonicalizeHandles(plan, dir, nil)
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

	findings, _, err := CanonicalizeHandles(plan, dir, results)
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

// TestRenameSignature covers the receiver-clause hazard: quarry's Symbol.Signature carries a
// method's receiver verbatim, so the first textual occurrence of the declared identifier is
// frequently inside the receiver TYPE rather than at the declared name.
func TestRenameSignature(t *testing.T) {
	cases := []struct {
		name      string
		signature string
		oldName   string
		newName   string
		want      string
		wantOK    bool
	}{
		{
			name:      "free function",
			signature: "func ToRename() int",
			oldName:   "ToRename",
			newName:   "Renamed",
			want:      "func Renamed() int",
			wantOK:    true,
		},
		{
			name:      "receiver type contains the method name as a substring",
			signature: "func (c *Counter) Count() int",
			oldName:   "Count",
			newName:   "Tally",
			want:      "func (c *Counter) Tally() int",
			wantOK:    true,
		},
		{
			name:      "receiver type equals the method name",
			signature: "func (r *Resolve) Resolve() error",
			oldName:   "Resolve",
			newName:   "Answer",
			want:      "func (r *Resolve) Answer() error",
			wantOK:    true,
		},
		{
			name:      "value receiver with type parameters",
			signature: "func (b Box[T]) Boxed() T",
			oldName:   "Boxed",
			newName:   "Wrapped",
			want:      "func (b Box[T]) Wrapped() T",
			wantOK:    true,
		},
		{
			name:      "interface method has no receiver clause",
			signature: "Read() (int, error)",
			oldName:   "Read",
			newName:   "Fetch",
			want:      "Fetch() (int, error)",
			wantOK:    true,
		},
		{
			name:      "parameter name merely contains the identifier",
			signature: "func Emit(emitter io.Writer) error",
			oldName:   "Emit",
			newName:   "Write",
			want:      "func Write(emitter io.Writer) error",
			wantOK:    true,
		},
		{
			name:      "identifier absent from the signature",
			signature: "func Other() int",
			oldName:   "Missing",
			newName:   "Renamed",
			wantOK:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := renameSignature(tc.signature, tc.oldName, tc.newName)
			if ok != tc.wantOK {
				t.Fatalf("renameSignature(%q, %q, %q) ok = %v; want %v", tc.signature, tc.oldName, tc.newName, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("renameSignature(%q, %q, %q) = %q; want %q", tc.signature, tc.oldName, tc.newName, got, tc.want)
			}
		})
	}
}

// TestDraftHandleIdentifier covers the qualified-member case: a method handle's member half is
// "Owner.Name", and only the Name half ever belongs in a declaration head.
func TestDraftHandleIdentifier(t *testing.T) {
	cases := []struct {
		handle string
		want   string
		wantOK bool
	}{
		{handle: "plan:internal/alpha#Renamed", want: "Renamed", wantOK: true},
		{handle: "plan:internal/alpha#Counter.Tally", want: "Tally", wantOK: true},
		{handle: "plan:internal/alpha#", wantOK: false},
		{handle: "plan:internal/alpha", wantOK: false},
	}

	for _, tc := range cases {
		got, ok := draftHandleIdentifier(tc.handle)
		if ok != tc.wantOK {
			t.Fatalf("draftHandleIdentifier(%q) ok = %v; want %v", tc.handle, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Errorf("draftHandleIdentifier(%q) = %q; want %q", tc.handle, got, tc.want)
		}
	}
}

// TestCanonicalizeHandles_RenameMethodDerivesAMethodDeclaration proves a method Rename pair
// canonicalizes end to end against a real repository. Before renameSignature this produced the
// declaration "func (c *Counter.Tallyer) Count() int", which quarry rejected as member_too_deep,
// so no method could be renamed through the glyph alphabet at all.
func TestCanonicalizeHandles_RenameMethodDerivesAMethodDeclaration(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\ntype Counter struct{ n int }\n\nfunc (c *Counter) Count() int { return c.n }\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Counter.Count"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Counter.Count` -> `plan:sub#Counter.Tally`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	findings, _, err := CanonicalizeHandles(plan, dir, results)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none — a method rename must canonicalize cleanly", findings)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#Counter.Tally") {
		t.Errorf("canonical method handle plan:sub#Counter.Tally missing after rewrite: %s", got)
	}
}

func TestCanonicalizeHandles_RenameOldUnresolved(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#DoesNotExist` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	// No result at all for "sub#DoesNotExist": the same as it never resolving found.
	findings, _, err := CanonicalizeHandles(plan, dir, nil)
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

	findings, _, err := CanonicalizeHandles(plan, dir, nil)
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

	findings, _, err := CanonicalizeHandles(plan, dir, nil)
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

	findings, _, err := CanonicalizeHandles(plan, dir, nil)
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

	findings, _, err := CanonicalizeHandles(plan, dir, nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if findings != nil {
		t.Errorf("findings = %+v; want nil under language: none", findings)
	}
}

// TestBindHandles_MatchedHandleRewritesDeclaringAndReferencingCard covers one handle bound from a
// matching Created symbol and rewritten on both the declaring and the referencing card.
func TestBindHandles_MatchedHandleRewritesDeclaringAndReferencingCard(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#New` -> `func New() {}`\n\n**Intent:** one\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#New"}}}}

	findings, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if strings.Contains(got1, "plan:sub#New") || !strings.Contains(got1, "sub#New") {
		t.Errorf("card 1 was not rewritten to the plain glyph: %s", got1)
	}
	if strings.Contains(got2, "plan:sub#New") || !strings.Contains(got2, "sub#New") {
		t.Errorf("card 2 (referencing) was not rewritten to the plain glyph: %s", got2)
	}

	// The declaring card's own bullet collapses to a plain ref rather than keeping the arrow.
	// Substituting in place left "`sub#New` -> `func New() {}`", which is no longer a handle
	// declaration but still carries the arrow, so the card parsed with a blocking handle-malformed
	// and an empty Create target list -- and every later begin-batch refused the plan.
	if !strings.Contains(got1, "- `sub#New`\n") {
		t.Errorf("card 1's declaration bullet did not collapse to a plain ref: %s", got1)
	}
	if strings.Contains(got1, "->") {
		t.Errorf("card 1 kept the declaration arrow after binding: %s", got1)
	}

	// The property that actually matters: the bound plan still parses, with the declaring card's
	// Create group naming the real glyph and reporting no finding of its own.
	reparsed, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan after binding returned error: %v", err)
	}
	for _, e := range planparser.ValidateFormat(reparsed, dir) {
		switch e.Check {
		case "handle-malformed", "card-field-empty", "handle-unreferenced", "handle-dangling":
			t.Errorf("bound plan reports %s: %s", e.Check, e.Detail)
		}
	}
	var createRefs []string
	for _, g := range reparsed.Cards[0].TargetGroups {
		if g.Type == planparser.CardTypeCreate {
			createRefs = append(createRefs, g.Refs...)
		}
	}
	if len(createRefs) != 1 || createRefs[0] != "sub#New" {
		t.Errorf("card 1's Create refs after binding = %v; want exactly [sub#New]", createRefs)
	}
}

// TestBindHandles_PartialMatchOnOneCardMismatchesAndRewritesNeither covers two handles on one card
// with only one delta match, producing bind-count-mismatch and rewriting neither.
func TestBindHandles_PartialMatchOnOneCardMismatchesAndRewritesNeither(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#One` -> `func One() {}`\n- `plan:sub#Two` -> `func Two() {}`\n\n**Intent:** one\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#One"}}}}

	findings, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "bind-count-mismatch" {
		t.Fatalf("findings = %+v; want exactly one bind-count-mismatch", findings)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#One") || !strings.Contains(got, "plan:sub#Two") {
		t.Errorf("card was rewritten despite the count mismatch: %s", got)
	}
}

// TestBindHandles_ZeroHandleCardNoFindingNoWrite covers a zero-handle card producing no finding
// and no write.
func TestBindHandles_ZeroHandleCardNoFindingNoWrite(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Edit:**\n- `sub/a.go`\n\n**Intent:** one\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	findings, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}
	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("card file was rewritten despite carrying zero handles:\nbefore: %s\nafter: %s", before, after)
	}
}

// TestBindHandles_SubstitutionReachesCardOutsideTheCompletedBatch covers the substitution reaching
// a card outside the completed batch: RewriteRefs rewrites the whole plan directory, so a
// referencing card not itself passed in cards still gets rewritten.
func TestBindHandles_SubstitutionReachesCardOutsideTheCompletedBatch(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#New` -> `func New() {}`\n\n**Intent:** one\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#New"}}}}

	// Only card 1 (the declaring card) is passed as the "completed batch" — card 2 is outside it.
	completedOnly := []planparser.Card{plan.Cards[0]}

	findings, err := BindHandles(plan, dir, delta, completedOnly)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got2 := readCardFile(t, dir, 2, "card2")
	if strings.Contains(got2, "plan:sub#New") {
		t.Errorf("card 2, outside the completed batch, was not reached by the plan-wide rewrite: %s", got2)
	}
}

// TestBindHandles_RenameNewSideHandleBinds is PG-2's own regression test (crucible round
// sonnet-xhigh-r8): a Rename-only card -- carrying no Create group, so its own Declarations is
// empty -- must still have its New-side handle bound once the rename lands, exactly as a Create
// declaration would be. Before this fix, BindHandles skipped any card with zero Declarations, so
// this card's own "plan:sub#New" handle never lost its prefix, permanently invisible to
// collectGlyphTargets and both containment tiers.
func TestBindHandles_RenameNewSideHandleBinds(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** rename\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	// The delta names the rename via Renamed, never Created — a rename is not a create, and a
	// BindHandles keyed only on delta.Created (as it was pre-fix) would never match this at all.
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}

	got1 := readCardFile(t, dir, 1, "card1")
	got2 := readCardFile(t, dir, 2, "card2")
	if strings.Contains(got1, "plan:sub#New") || !strings.Contains(got1, "sub#New") {
		t.Errorf("card 1's own Rename pair's New side was not bound to the plain glyph: %s", got1)
	}
	if strings.Contains(got2, "plan:sub#New") || !strings.Contains(got2, "sub#New") {
		t.Errorf("card 2 (referencing the Rename's New side) was not rewritten to the plain glyph: %s", got2)
	}

	// The bound plan still parses, and the reference is now a genuine glyph -- reachable by
	// collectGlyphTargets and both containment tiers, which exclude anything plan:-prefixed by
	// construction.
	reparsed, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan after binding returned error: %v", err)
	}
	for _, e := range planparser.ValidateFormat(reparsed, dir) {
		switch e.Check {
		case "handle-malformed", "handle-unreferenced", "handle-dangling":
			t.Errorf("bound plan reports %s: %s", e.Check, e.Detail)
		}
	}
}

// TestBindHandles_RenameFileSidePairIsNotAHandle covers a file-rename pair (both sides self glyphs,
// per spec's own exemption) producing no finding and no write: neither side is handle-shaped, so
// cardOwnHandles reports nothing to bind and the card is skipped exactly like a zero-handle card.
func TestBindHandles_RenameFileSidePairIsNotAHandle(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub/old.go` -> `sub/new.go`\n\n**Intent:** rename a file\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	findings, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}
	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("file-rename card was rewritten despite carrying no handle:\nbefore: %s\nafter: %s", before, after)
	}
}

// TestBindHandles_RenameNewSideUnmatchedMismatches covers a Rename's own New-side handle whose
// expected glyph the delta's Renamed set does not carry: bind-count-mismatch, rewriting neither
// side, exactly as an unmatched Create declaration does.
func TestBindHandles_RenameNewSideUnmatchedMismatches(t *testing.T) {
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** rename\n",
	})
	// An unrelated rename in the delta — its own To.ID does not match this card's expected glyph.
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "other#A"}, To: quarry.Symbol{ID: "other#B"}}},
	}}

	findings, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "bind-count-mismatch" {
		t.Fatalf("findings = %+v; want exactly one bind-count-mismatch", findings)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#New") {
		t.Errorf("card was rewritten despite the mismatch: %s", got)
	}
}

// TestBindHandles_LanguageNoneNoOp mirrors CanonicalizeHandles' own no-op under language: none.
func TestBindHandles_LanguageNoneNoOp(t *testing.T) {
	dir := t.TempDir()
	plan := &planparser.Plan{Dir: dir, Language: "none"}

	findings, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, nil)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if findings != nil {
		t.Errorf("findings = %+v; want nil under language: none", findings)
	}
}
