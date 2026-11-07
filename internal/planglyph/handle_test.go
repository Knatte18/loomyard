// handle_test.go covers CanonicalizeHandles and BindHandles.
// The CanonicalizeHandles tests check the batched Name call and positional matching.
// They also check the Rename-derived source, failure and collision handling, and the on-disk rewrite.

package planglyph

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// TestCanonicalizeHandles_Rewrites covers CanonicalizeHandles.
// CanonicalizeHandles rewrites the draft handles of a plan into their canonical spelling without a finding.
// Create declarations and a Rename-derived declaration ride one batched call.
// Each declaration is matched back to its own handle by position,
// so a handle sorting after another's whose declared identifier differs from its member name must not be cross-wired.
// The rewrite lands on every card referencing the handle.
// A Rename's unit comes from the old side's resolved glyph, never from the draft,
// so a misspelled unit does not survive.
// A method rename derives a method declaration with a receiver quarry accepts;
// a receiver naming a dotted member path is rejected as member_too_deep.
func TestCanonicalizeHandles_Rewrites(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	oldFoo := map[string]string{"sub/a.go": "package sub\n\nfunc Old() {}\n"}

	cases := []struct {
		name string
		// files, when set, is the repository the targets of resolve are answered against.
		files   map[string]string
		resolve []string
		cards   map[int]string
		// wantPresent and wantAbsent map a card number to substrings its file must and must not contain afterwards.
		wantPresent map[int][]string
		wantAbsent  map[int][]string
	}{
		{
			// Each Create draft handle already names its computed identifier verbatim (plan:sub#A -> func A() {}).
			// So canonicalization is a no-op rewrite for those three.
			// The case checks that all four sources ride one batched call and that none reports a finding.
			name:    "batched call covers both sources",
			files:   oldFoo,
			resolve: []string{"sub#Old"},
			cards: map[int]string{
				1: "**Create:**\n- `plan:sub#A` -> `func A() {}`\n\n**Intent:** one\n",
				2: "**Create:**\n- `plan:sub#B` -> `func B() {}`\n\n**Intent:** two\n",
				3: "**Create:**\n- `plan:sub#C` -> `func C() {}`\n\n**Intent:** three\n",
				4: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** four\n\n## Rename mechanic\n",
			},
			wantPresent: map[int][]string{1: {"plan:sub#"}, 2: {"plan:sub#"}, 3: {"plan:sub#"}, 4: {"plan:sub#"}},
		},
		{
			// Card 1's handle sorts after card 2's ("Z" > "A"),
			// and each declared identifier differs from its own handle's member name:
			// a positional mismatch in matching Name's results back to their sources would cross-wire these two,
			// and the rewrite would come out detectably wrong.
			name: "positional matching out of order",
			cards: map[int]string{
				1: "**Create:**\n- `plan:sub#Z` -> `func ActualZ() {}`\n\n**Intent:** one\n",
				2: "**Create:**\n- `plan:sub#A` -> `func ActualA() {}`\n\n**Intent:** two\n",
			},
			wantPresent: map[int][]string{1: {"plan:sub#ActualZ"}, 2: {"plan:sub#ActualA"}},
		},
		{
			// The draft to-side misspells the unit half ("wrongpkg" instead of "sub", where the old side resolved):
			// canonicalization derives the Unit from Old's own resolved glyph, never from the draft,
			// so the wrong unit must not survive the rewrite.
			name:    "rename draft spelling wrong still rewrites",
			files:   oldFoo,
			resolve: []string{"sub#Old"},
			cards: map[int]string{
				1: "**Rename:**\n- `sub#Old` -> `plan:wrongpkg#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
			},
			wantPresent: map[int][]string{1: {"plan:sub#New"}},
			wantAbsent:  map[int][]string{1: {"plan:wrongpkg#New"}},
		},
		{
			name: "rename of a method derives a method declaration",
			files: map[string]string{
				"sub/a.go": "package sub\n\ntype Counter struct{ n int }\n\nfunc (c *Counter) Count() int { return c.n }\n",
			},
			resolve: []string{"sub#Counter.Count"},
			cards: map[int]string{
				1: "**Rename:**\n- `sub#Counter.Count` -> `plan:sub#Counter.Tally`\n\n**Intent:** one\n\n## Rename mechanic\n",
			},
			wantPresent: map[int][]string{1: {"plan:sub#Counter.Tally"}},
		},
		{
			// The declared identifier ("ActualNew") differs from the draft handle's own member name ("New"),
			// so the rewrite is verifiable against a changed string on both the declaring card and the referencing card.
			name: "rewrite lands on every referencing card",
			cards: map[int]string{
				1: "**Create:**\n- `plan:sub#New` -> `func ActualNew() {}`\n\n**Intent:** one\n",
				2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
			},
			wantAbsent: map[int][]string{1: {"plan:sub#New"}, 2: {"plan:sub#New"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var results []quarry.ResolveResult
			root := ""
			if tc.files != nil {
				root = writeFixtureRepo(t, tc.files)
				repo, err := openRepo(root)
				if err != nil {
					t.Fatalf("openRepo(%q) returned error: %v", root, err)
				}
				results, err = resolveTargets(repo, tc.resolve)
				if err != nil {
					t.Fatalf("resolveTargets(...) returned error: %v", err)
				}
			}
			dir, plan := writePlanFixture(t, tc.cards)
			// plan.Dir must equal dir, as it does for the plan a production caller passes; ParsePlan already stamps it.
			if plan.Dir != dir {
				t.Fatalf("plan.Dir = %q; want %q", plan.Dir, dir)
			}

			findings, _, err := CanonicalizeHandles(plan, root, results)
			if err != nil {
				t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("findings = %+v; want none", findings)
			}

			for n, wants := range tc.wantPresent {
				got := readCardFile(t, dir, n, fmt.Sprintf("card%d", n))
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("card %d lacks %q after canonicalization: %s", n, want, got)
					}
				}
			}
			for n, unwanted := range tc.wantAbsent {
				got := readCardFile(t, dir, n, fmt.Sprintf("card%d", n))
				for _, bad := range unwanted {
					if strings.Contains(got, bad) {
						t.Errorf("card %d still carries %q after canonicalization: %s", n, bad, got)
					}
				}
			}
		})
	}
}

// TestCanonicalizeHandles_OneDraftTwoCanonicalsRewritesNothing covers one draft handle that two sources claim:
// a Create declaration (unit from the handle itself) and a Rename to-side (unit from the resolved old side).
// The two canonicalize to different glyphs.
// A plan like that is refused by the blocking pure handle-collision finding.
// The resolve layer must not pick one canonical by map-iteration accident:
// it would rewrite the plan on disk with that canonical before the refusal is rendered.
func TestCanonicalizeHandles_OneDraftTwoCanonicalsRewritesNothing(t *testing.T) {
	tiergate.Require(t, 0)
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
		1: "**Create:**\n- `plan:other#New` -> `func New() {}`\n\n**Intent:** one\n",
		2: "**Rename:**\n- `sub#Old` -> `plan:other#New`\n\n**Intent:** two\n\n## Rename mechanic\n",
	})
	before1 := readCardFile(t, dir, 1, "card1")
	before2 := readCardFile(t, dir, 2, "card2")

	findings, rewrote, err := CanonicalizeHandles(plan, root, results)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	for _, f := range findings {
		t.Errorf("unexpected finding: %+v — the refusal is the pure handle-collision finding's, not this layer's", f)
	}
	if rewrote {
		t.Errorf("CanonicalizeHandles(...) rewrote = true; want false — an arbitrarily picked canonical must never land on disk")
	}
	if got := readCardFile(t, dir, 1, "card1"); got != before1 {
		t.Errorf("card 1 was rewritten:\nbefore: %s\nafter: %s", before1, got)
	}
	if got := readCardFile(t, dir, 2, "card2"); got != before2 {
		t.Errorf("card 2 was rewritten:\nbefore: %s\nafter: %s", before2, got)
	}
}

// TestRenameSignature covers the receiver-clause hazard.
// quarry's Symbol.Signature carries a method's receiver verbatim,
// so the first textual occurrence of the declared identifier is frequently in the receiver TYPE, not at its declaration.
func TestRenameSignature(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRenameCSharpDecl pins the exact heads a C# rename derives.
// The glyph-chain rows see those heads only through the minted id.
func TestRenameCSharpDecl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		kind       quarry.Kind
		signature  string
		symbolName string
		identifier string
		want       string
		wantOK     bool
	}{
		{"ConstKeepsOneDeclarator", quarry.KindConst, "public const int A = 1, B = 2;", "A", "C", "public const int C = default", true},
		{"FieldKeepsGenericType", quarry.KindField, "private Dictionary<string, int> map;", "map", "renamed", "private Dictionary<string, int> renamed", true},
		{"FieldKeepsTupleType", quarry.KindField, "private (int, int) pair;", "pair", "twin", "private (int, int) twin", true},
		{"PropertyWhoseTypeIsGenericOfItsNameReplacesTheType", quarry.KindProperty, "Color<int> Color { get; }", "Color", "Hue", "Hue<int> Color { get; }", true},
		{"OperatorHasNoDeclaredName", quarry.KindOperator, "public static Box operator +(Box a, Box b)", "operator+", "Plus", "", false},
		{"FinalizerHasNoDeclaredName", quarry.KindMethod, "~Renderer()", "Renderer", "Other", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sym := quarry.Symbol{Kind: tt.kind, Signature: tt.signature, Glyph: glyph.Glyph{Lang: glyph.CSharp, Name: tt.symbolName}}
			got, ok := renameCSharpDecl(sym, tt.identifier)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("renameCSharpDecl(%q, %q) = %q, %v; want %q, %v", tt.signature, tt.identifier, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRenameCSharpDecl_ConstructorIsNamedByItsType(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		owner     string
		symbol    string
		signature string
	}{
		{"Plain", "Bar", "Bar", "public Bar(int x)"},
		{"GenericOwner", "Bar<>", "Bar<>", "public Bar(T x)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sym := quarry.Symbol{
				Kind:      quarry.KindMethod,
				Signature: tt.signature,
				Glyph:     glyph.Glyph{Lang: glyph.CSharp, Owner: []string{tt.owner}, Name: tt.symbol},
			}
			if got, ok := renameCSharpDecl(sym, "Other"); ok {
				t.Errorf("renameCSharpDecl(%q, %q) = %q, true; want false", tt.signature, "Other", got)
			}
		})
	}
}

func TestCanonicalizeHandles_RenameOldUnresolved(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#DoesNotExist` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	// No result at all for "sub#DoesNotExist" counts the same as a glyph that never resolves to found.
	findings, _, err := CanonicalizeHandles(plan, "", nil)
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

	// An old side declared once per build-constraint set stays unresolved,
	// and its finding names the candidates and the routes that work.
	t.Run("old side partitioned by build constraints", func(t *testing.T) {
		root := writeFixtureRepo(t, map[string]string{
			"sub/a.go": "//go:build linux\n\npackage sub\n\nfunc Old() {}\n",
			"sub/b.go": "//go:build !linux\n\npackage sub\n\nfunc Old() {}\n",
		})
		repo, err := openRepo(root)
		if err != nil {
			t.Fatalf("openRepo(%q) returned error: %v", root, err)
		}
		results, err := resolveTargets(repo, []string{"sub#Old"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != quarry.StatusAmbiguous {
			t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, quarry.StatusAmbiguous)
		}
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
		})

		findings, rewrote, err := CanonicalizeHandles(plan, root, results)
		if err != nil {
			t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
		}
		if len(findings) != 1 || findings[0].Check != "rename-old-unresolved" {
			t.Fatalf("findings = %+v; want exactly one rename-old-unresolved finding", findings)
		}
		for _, want := range []string{
			"sub/a.go, //go:build linux", "sub/b.go, //go:build !linux",
			"delete the old member and create the new member's handle once",
			"or edit each file's body",
			"a Create per file is no way forward",
		} {
			if !strings.Contains(findings[0].Detail, want) {
				t.Errorf("finding detail = %q; want it to contain %q", findings[0].Detail, want)
			}
		}
		if rewrote {
			t.Errorf("CanonicalizeHandles(...) rewrote = true; want false for an unresolved old side")
		}
		if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "plan:sub#New") {
			t.Errorf("card was rewritten despite an unresolved old side: %s", got)
		}
	})
}

// TestCanonicalizeHandles_RenameOldSelfGlyphNamesTheShapeMistake covers a found answer with no Symbols.
// That answer belongs to a self glyph, a file or unit rather than a symbol,
// so the finding must say so instead of claiming the old side "did not resolve found".
// That claim is false for a self glyph and sends the operator at a resolution problem.
func TestCanonicalizeHandles_RenameOldSelfGlyphNamesTheShapeMistake(t *testing.T) {
	tiergate.Require(t, 0)
	_, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub/a.go#` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})

	// A found self glyph's answer carries a Listing and no Symbols;
	// only the Symbols absence matters to renameDeclSource.
	results := []quarry.ResolveResult{{Target: "sub/a.go#", Status: quarry.StatusFound}}
	findings, _, err := CanonicalizeHandles(plan, "", results)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "rename-old-unresolved" {
		t.Fatalf("findings = %+v; want exactly one rename-old-unresolved finding", findings)
	}
	if !strings.Contains(findings[0].Detail, "names a file or unit, not a symbol") {
		t.Errorf("finding detail = %q; want it to name the shape mistake, not a resolution failure", findings[0].Detail)
	}
	if strings.Contains(findings[0].Detail, "did not resolve found") {
		t.Errorf("finding detail = %q; must not claim the old side did not resolve found — it did", findings[0].Detail)
	}
}

func TestCanonicalizeHandles_OneFailingDeclarationLeavesOthersRewritten(t *testing.T) {
	tiergate.Require(t, 0)
	// Card 1's draft handle misspells the declared identifier ("Good" versus the declaration head's own "ActualGood"),
	// so its successful rewrite is verifiable against a changed string, not merely a no-op round trip.
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#Good` -> `func ActualGood() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#Bad` -> `this is not valid go at all {{{`\n\n**Intent:** two\n",
	})

	findings, _, err := CanonicalizeHandles(plan, "", nil)
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
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#One` -> `func Same() {}`\n\n**Intent:** one\n",
		2: "**Create:**\n- `plan:sub#Two` -> `func Same() {}`\n\n**Intent:** two\n",
	})

	findings, _, err := CanonicalizeHandles(plan, "", nil)
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

func TestCanonicalizeHandles_LanguageNoneNoOp(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	plan := &planparser.Plan{Dir: dir, Language: "none"}

	findings, _, err := CanonicalizeHandles(plan, "", nil)
	if err != nil {
		t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
	}
	if findings != nil {
		t.Errorf("findings = %+v; want nil under language: none", findings)
	}
}

// TestBindHandles_MatchedHandleRewritesDeclaringAndReferencingCard covers one handle.
// The handle is bound from a matching Created symbol and rewritten on both the declaring and the referencing card.
func TestBindHandles_MatchedHandleRewritesDeclaringAndReferencingCard(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#New` -> `func New() {}`\n\n**Intent:** one\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#New"}}}}

	findings, outcomes, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}
	if want := []planindex.BindOutcome{{Card: 1, Handle: "plan:sub#New", Glyph: "sub#New"}}; !reflect.DeepEqual(outcomes, want) {
		t.Errorf("outcomes = %+v; want %+v", outcomes, want)
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
	// Substituting in place would leave "`sub#New` -> `func New() {}`", a bullet with an arrow but no handle declaration.
	// The card would then parse with a blocking handle-malformed finding and an empty Create target list,
	// and every later begin-batch would refuse the plan.
	if !strings.Contains(got1, "- `sub#New`\n") {
		t.Errorf("card 1's declaration bullet did not collapse to a plain ref: %s", got1)
	}
	if strings.Contains(got1, "->") {
		t.Errorf("card 1 kept the declaration arrow after binding: %s", got1)
	}

	// The property that matters: the bound plan still parses.
	// The declaring card's Create group names the real glyph and reports no finding of its own.
	reparsed, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan after binding returned error: %v", err)
	}
	for _, e := range planparser.ValidateFormat(reparsed, dir) {
		switch e.Check {
		case "handle-malformed", "card-field-empty", "handle-dangling":
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

// TestBindHandles_HandleBindsToTheOtherPackageOfItsDirectory covers a handle whose test lives in the other package.
// The handle binds to the glyph the delta created there, in either direction,
// and the plan is rewritten to that glyph.
func TestBindHandles_HandleBindsToTheOtherPackageOfItsDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		handle string
		// delta lists the IDs of the symbols the delta created.
		delta []string
		// created is the glyph the handle is expected to bind to.
		created string
	}{
		{name: "package handle created in the external test package", handle: "plan:sub#TestNew", delta: []string{"sub_test#TestNew"}, created: "sub_test#TestNew"},
		{name: "external test handle created in the package", handle: "plan:sub_test#TestNew", delta: []string{"sub#TestNew"}, created: "sub#TestNew"},
		{name: "exact match wins over the other package", handle: "plan:sub#TestNew", delta: []string{"sub_test#TestNew", "sub#TestNew"}, created: "sub#TestNew"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, plan := writePlanFixture(t, map[int]string{
				1: "**Create:**\n- `" + tt.handle + "` -> `func TestNew(t *testing.T) {}`\n\n**Intent:** one\n",
			})
			var created []quarry.Symbol
			for _, id := range tt.delta {
				created = append(created, quarry.Symbol{ID: id})
			}
			delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: created}}

			findings, outcomes, err := BindHandles(plan, dir, delta, plan.Cards)
			if err != nil {
				t.Fatalf("BindHandles(...) returned error: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("findings = %+v; want none", findings)
			}
			if want := []planindex.BindOutcome{{Card: 1, Handle: tt.handle, Glyph: tt.created}}; !reflect.DeepEqual(outcomes, want) {
				t.Errorf("outcomes = %+v; want %+v", outcomes, want)
			}
			if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "- `"+tt.created+"`\n") {
				t.Errorf("card 1 was not rewritten to %s: %s", tt.created, got)
			}
		})
	}
}

// TestBindHandles_OtherPackageMatchStillCountsTowardMismatch covers a card whose handle only the other package matches.
// The match still counts toward the mismatch check.
// A card with a handle left unmatched, or reaching an already bound glyph, reports `bind-count-mismatch`.
// It rewrites nothing.
func TestBindHandles_OtherPackageMatchStillCountsTowardMismatch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name         string
		handles      []string
		created      string
		wantOutcomes []planindex.BindOutcome
	}{
		{
			name:    "one handle matched through the other package and one unmatched",
			handles: []string{"plan:sub#TestOne", "plan:sub#TestTwo"},
			created: "sub_test#TestOne",
			wantOutcomes: []planindex.BindOutcome{
				{Card: 1, Handle: "plan:sub#TestOne", Reason: planindex.BindSiblingUnmatched},
				{Card: 1, Handle: "plan:sub#TestTwo", Reason: planindex.BindNoDeltaMatch},
			},
		},
		{
			name:    "both spellings of one member reaching a single created symbol",
			handles: []string{"plan:sub#TestX", "plan:sub_test#TestX"},
			created: "sub_test#TestX",
			wantOutcomes: []planindex.BindOutcome{
				{Card: 1, Handle: "plan:sub#TestX", Reason: planindex.BindSiblingUnmatched},
				{Card: 1, Handle: "plan:sub_test#TestX", Reason: planindex.BindNoDeltaMatch},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var body strings.Builder
			body.WriteString("**Create:**\n")
			for _, h := range tt.handles {
				body.WriteString("- `" + h + "` -> `func Stub() {}`\n")
			}
			body.WriteString("\n**Intent:** one\n")
			dir, plan := writePlanFixture(t, map[int]string{1: body.String()})
			delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: tt.created}}}}

			findings, outcomes, err := BindHandles(plan, dir, delta, plan.Cards)
			if err != nil {
				t.Fatalf("BindHandles(...) returned error: %v", err)
			}
			if len(findings) != 1 || findings[0].Check != "bind-count-mismatch" {
				t.Fatalf("findings = %+v; want exactly one bind-count-mismatch", findings)
			}
			if !reflect.DeepEqual(outcomes, tt.wantOutcomes) {
				t.Errorf("outcomes = %+v; want %+v", outcomes, tt.wantOutcomes)
			}
			got := readCardFile(t, dir, 1, "card1")
			for _, h := range tt.handles {
				if !strings.Contains(got, h) {
					t.Errorf("card was rewritten despite the count mismatch, %s is gone: %s", h, got)
				}
			}
		})
	}
}

// TestBindHandles_PartialMatchOnOneCardMismatchesAndRewritesNeither covers two handles on one card with one delta match.
// The card produces `bind-count-mismatch` and rewrites neither handle.
func TestBindHandles_PartialMatchOnOneCardMismatchesAndRewritesNeither(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#One` -> `func One() {}`\n- `plan:sub#Two` -> `func Two() {}`\n\n**Intent:** one\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#One"}}}}

	findings, outcomes, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "bind-count-mismatch" {
		t.Fatalf("findings = %+v; want exactly one bind-count-mismatch", findings)
	}
	wantOutcomes := []planindex.BindOutcome{
		{Card: 1, Handle: "plan:sub#One", Reason: planindex.BindSiblingUnmatched},
		{Card: 1, Handle: "plan:sub#Two", Reason: planindex.BindNoDeltaMatch},
	}
	if !reflect.DeepEqual(outcomes, wantOutcomes) {
		t.Errorf("outcomes = %+v; want %+v", outcomes, wantOutcomes)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#One") || !strings.Contains(got, "plan:sub#Two") {
		t.Errorf("card was rewritten despite the count mismatch: %s", got)
	}
}

// TestBindHandles_ZeroHandleCardNoFindingNoWrite covers a card with no handles.
// It produces no finding and rewrites nothing.
func TestBindHandles_ZeroHandleCardNoFindingNoWrite(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Edit:**\n- `sub/a.go`\n\n**Intent:** one\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	findings, outcomes, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 || len(outcomes) != 0 {
		t.Fatalf("findings = %+v, outcomes = %+v; want none", findings, outcomes)
	}
	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("card file was rewritten despite carrying zero handles:\nbefore: %s\nafter: %s", before, after)
	}
}

// TestBindHandles_SubstitutionReachesCardOutsideTheCompletedBatch covers a rewrite reaching a card outside the batch.
// `RewriteRefs` rewrites the whole plan directory, so a referencing card outside the batch still gets rewritten.
func TestBindHandles_SubstitutionReachesCardOutsideTheCompletedBatch(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Create:**\n- `plan:sub#New` -> `func New() {}`\n\n**Intent:** one\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#New"}}}}

	// Only card 1, the declaring card, is passed as the completed batch; card 2 lies outside it.
	completedOnly := []planparser.Card{plan.Cards[0]}

	findings, _, err := BindHandles(plan, dir, delta, completedOnly)
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

// TestBindHandles_RenameNewSideHandleBinds covers a card with only a Rename and no Create group.
// Its New-side handle must still bind once the rename lands, as a Create declaration's handle does.
func TestBindHandles_RenameNewSideHandleBinds(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** rename\n",
		2: "**Uses:**\n- `plan:sub#New`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n",
	})
	// The delta names the rename in Renamed, never in Created: a rename is not a create.
	// A BindHandles keyed only on `delta.Created` would never match this handle.
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, _, err := BindHandles(plan, dir, delta, plan.Cards)
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

	// The bound plan still parses.
	// The reference is now a genuine glyph.
	// `collectGlyphTargets` reaches it, because that function excludes `plan:`-prefixed refs by construction.
	reparsed, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan after binding returned error: %v", err)
	}
	for _, e := range planparser.ValidateFormat(reparsed, dir) {
		switch e.Check {
		case "handle-malformed", "handle-dangling":
			t.Errorf("bound plan reports %s: %s", e.Check, e.Detail)
		}
	}
}

// TestBindHandles_RenameFileSidePairIsNotAHandle covers a file-rename pair.
// Both sides are self glyphs, which the spec exempts from handles.
// Neither side is handle-shaped, so `planparser.OwnedHandles` reports nothing to bind,
// and the card produces no finding and no write.
func TestBindHandles_RenameFileSidePairIsNotAHandle(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub/old.go` -> `sub/new.go`\n\n**Intent:** rename a file\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	findings, _, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, plan.Cards)
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

// TestBindHandles_RenameNewSideUnmatchedMismatches covers a rename's New-side handle with no Renamed match.
// The delta's Renamed set lacks the handle's expected glyph, so the card reports `bind-count-mismatch`,
// and it rewrites neither side.
func TestBindHandles_RenameNewSideUnmatchedMismatches(t *testing.T) {
	tiergate.Require(t, 0)
	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** rename\n",
	})
	// An unrelated rename in the delta: its To.ID is not the expected glyph.
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "other#A"}, To: quarry.Symbol{ID: "other#B"}}},
	}}

	findings, outcomes, err := BindHandles(plan, dir, delta, plan.Cards)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "bind-count-mismatch" {
		t.Fatalf("findings = %+v; want exactly one bind-count-mismatch", findings)
	}
	if want := []planindex.BindOutcome{{Card: 1, Handle: "plan:sub#New", Reason: planindex.BindNoDeltaMatch}}; !reflect.DeepEqual(outcomes, want) {
		t.Errorf("outcomes = %+v; want %+v", outcomes, want)
	}

	got := readCardFile(t, dir, 1, "card1")
	if !strings.Contains(got, "plan:sub#New") {
		t.Errorf("card was rewritten despite the mismatch: %s", got)
	}
}

// TestBindHandles_LanguageNoneNoOp covers a plan whose language is none: binding returns nil findings and outcomes.
func TestBindHandles_LanguageNoneNoOp(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	plan := &planparser.Plan{Dir: dir, Language: "none"}

	findings, outcomes, err := BindHandles(plan, dir, quarry.GitDeltaAnswer{}, nil)
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if findings != nil || outcomes != nil {
		t.Errorf("findings = %+v, outcomes = %+v; want nil under language: none", findings, outcomes)
	}
}
