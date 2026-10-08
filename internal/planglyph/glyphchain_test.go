// glyphchain_test.go drives every member shape class real plans name through the glyph chain's plan gate against the committed fixture module in testdata/glyphchain: parse, the resolve status policy, the Create inversion, handle canonicalization and the on-disk rewrite.
// The legs here read files and spawn no process; the legs that need a real commit's delta live in glyphchain_integration_test.go beside them.

package planglyph

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// findingKey is the part of a Finding a chain leg asserts: which check fired on which card at what severity.
type findingKey struct {
	check    string
	card     string
	severity Severity
}

// createLeg is a Create leg: a draft handle declared by head, and the canonical handle quarry derives from head.
type createLeg struct {
	draft     string
	head      string
	canonical string
}

// renameLeg is a Rename leg: the draft to-side handle, spelled in the wrong unit, and the canonical handle in the Old glyph's unit.
type renameLeg struct {
	draft     string
	canonical string
}

// codeEdit is one textual change to a fixture file: old is replaced by new, and an empty old appends new to the file.
type codeEdit struct {
	path string
	old  string
	new  string
}

func appendText(path, text string) codeEdit { return codeEdit{path: path, new: text} }

func replaceText(path, old, new string) codeEdit { return codeEdit{path: path, old: old, new: new} }

// shapeRow is one member shape class and the legs the chain runs on it; a nil leg is not run for the row.
type shapeRow struct {
	name  string
	glyph string
	// editFindings is the exact finding set of a card editing the glyph.
	editFindings []findingKey
	create       *createLeg
	rename       *renameLeg
	// deleteGlyphs are the targets of the Delete leg's one card.
	deleteGlyphs []string
	// createEdits, renameEdits and deleteEdits are the code change each leg's card describes, applied to a fixture copy by the legs that need a real commit.
	createEdits []codeEdit
	renameEdits []codeEdit
	deleteEdits []codeEdit
	// gap names a known defect the row's assertions record rather than fix.
	gap string
}

// newRenameLeg returns the Rename leg of glyph: a to-side handle carrying member name plus "Renamed" in wrongUnit, and the canonical handle in glyph's own unit.
func newRenameLeg(glyph, wrongUnit string) *renameLeg {
	unit, member, _ := strings.Cut(glyph, "#")
	return &renameLeg{
		draft:     fmt.Sprintf("plan:%s#%sRenamed", wrongUnit, member),
		canonical: fmt.Sprintf("plan:%s#%sRenamed", unit, member),
	}
}

const (
	shapesFile       = "shapes/shapes.go"
	shapesTestFile   = "shapes/shapes_test.go"
	externalTestFile = "shapes/external_test.go"
	taggedFile       = "shapes/tagged.go"
	clauseFile       = "dirname/clause.go"
	mainFile         = "cmd/tool/main.go"
)

// shapeRows lists one row per member shape class real plans name.
func shapeRows() []shapeRow {
	rows := []shapeRow{
		{
			glyph:       "shapes#Func",
			create:      &createLeg{"plan:shapes#draft", "func NewFunc() int", "plan:shapes#NewFunc"},
			createEdits: []codeEdit{appendText(shapesFile, "\nfunc NewFunc() int { return 1 }\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "func Func() int", "func FuncRenamed() int")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "func Func() int { return 0 }\n", "")},
		},
		{
			glyph:       "shapes#Plain",
			create:      &createLeg{"plan:shapes#draft", "type NewPlain struct{}", "plan:shapes#NewPlain"},
			createEdits: []codeEdit{appendText(shapesFile, "\ntype NewPlain struct{}\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "type Plain struct{}", "type PlainRenamed struct{}")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "type Plain struct{}\n", "")},
		},
		{
			glyph:       "shapes#Var",
			create:      &createLeg{"plan:shapes#draft", "var NewVar = 1", "plan:shapes#NewVar"},
			createEdits: []codeEdit{appendText(shapesFile, "\nvar NewVar = 1\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "var Var = 1", "var VarRenamed = 1")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "var Var = 1\n", "")},
		},
		{
			glyph:       "shapes#Const",
			create:      &createLeg{"plan:shapes#draft", "const NewConst = 2", "plan:shapes#NewConst"},
			createEdits: []codeEdit{appendText(shapesFile, "\nconst NewConst = 2\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "const Const = 2", "const ConstRenamed = 2")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "const Const = 2\n", "")},
		},
		{
			glyph:       "shapes#Struct.ValueMethod",
			create:      &createLeg{"plan:shapes#Struct.draft", "func (s Struct) NewValueMethod() int", "plan:shapes#Struct.NewValueMethod"},
			createEdits: []codeEdit{appendText(shapesFile, "\nfunc (s Struct) NewValueMethod() int { return 1 }\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "ValueMethod() int", "ValueMethodRenamed() int")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "func (s Struct) ValueMethod() int { return s.Field }\n", "")},
		},
		{
			glyph:       "shapes#Struct.PointerMethod",
			create:      &createLeg{"plan:shapes#Struct.draft", "func (s *Struct) NewPointerMethod()", "plan:shapes#Struct.NewPointerMethod"},
			createEdits: []codeEdit{appendText(shapesFile, "\nfunc (s *Struct) NewPointerMethod() {}\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "PointerMethod() {", "PointerMethodRenamed() {")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "func (s *Struct) PointerMethod() { s.Field++ }\n", "")},
		},
		{
			glyph: "shapes#Struct.Field",
			// quarry indexes no struct fields, so a field glyph never resolves.
			editFindings: []findingKey{{"glyph-not-found", "1-card1", SeverityBlocking}},
			gap:          "quarry indexes no struct fields; shapes#Struct.Field resolves not_found",
		},
		{
			glyph:       "shapes#Iface.IfaceMethod",
			create:      &createLeg{"plan:shapes#Iface.draft", "func (i Iface) NewIfaceMethod() int", "plan:shapes#Iface.NewIfaceMethod"},
			createEdits: []codeEdit{replaceText(shapesFile, "IfaceMethod() int\n}", "IfaceMethod() int\n\tNewIfaceMethod() int\n}")},
			renameEdits: []codeEdit{replaceText(shapesFile, "IfaceMethod() int\n}", "IfaceMethodRenamed() int\n}")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "\tIfaceMethod() int\n", "")},
		},
		{
			// The method cannot outlive its receiver type, so the Delete leg's card deletes both, and the rename carries the method's receiver along.
			glyph:       "shapes#Generic",
			create:      &createLeg{"plan:shapes#draft", "type NewGeneric[E any] struct{}", "plan:shapes#NewGeneric"},
			createEdits: []codeEdit{appendText(shapesFile, "\ntype NewGeneric[E any] struct{}\n")},
			renameEdits: []codeEdit{
				replaceText(shapesFile, "type Generic[E any]", "type GenericRenamed[E any]"),
				replaceText(shapesFile, "func (g *Generic[E])", "func (g *GenericRenamed[E])"),
			},
			deleteGlyphs: []string{"shapes#Generic", "shapes#Generic.GenericMethod"},
			deleteEdits: []codeEdit{
				replaceText(shapesFile, "type Generic[E any] struct{ value E }\n", ""),
				replaceText(shapesFile, "func (g *Generic[E]) GenericMethod() E { return g.value }\n", ""),
			},
		},
		{
			glyph:       "shapes#Generic.GenericMethod",
			create:      &createLeg{"plan:shapes#Generic.draft", "func (g *Generic[E]) NewGenericMethod() E", "plan:shapes#Generic.NewGenericMethod"},
			createEdits: []codeEdit{appendText(shapesFile, "\nfunc (g *Generic[E]) NewGenericMethod() E { return g.value }\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "GenericMethod() E", "GenericMethodRenamed() E")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "func (g *Generic[E]) GenericMethod() E { return g.value }\n", "")},
		},
		{
			glyph:       "shapes#testHelper",
			create:      &createLeg{"plan:shapes#draft", "func newTestHelper() int", "plan:shapes#newTestHelper"},
			createEdits: []codeEdit{appendText(shapesTestFile, "\nfunc newTestHelper() int { return 0 }\n")},
			renameEdits: []codeEdit{replaceText(shapesTestFile, "func testHelper()", "func testHelperRenamed()")},
			deleteEdits: []codeEdit{replaceText(shapesTestFile, "func testHelper() int { return 0 }\n", "")},
		},
		{
			glyph:       "shapes_test#TestExternal",
			create:      &createLeg{"plan:shapes_test#draft", "func TestNewExternal(t *testing.T)", "plan:shapes_test#TestNewExternal"},
			createEdits: []codeEdit{appendText(externalTestFile, "\nfunc TestNewExternal(t *testing.T) {}\n")},
			renameEdits: []codeEdit{replaceText(externalTestFile, "func TestExternal(", "func TestExternalRenamed(")},
			deleteEdits: []codeEdit{replaceText(externalTestFile, "func TestExternal(t *testing.T) {}\n", "")},
		},
		{
			glyph:       "shapes#Tagged",
			create:      &createLeg{"plan:shapes#draft", "func NewTagged()", "plan:shapes#NewTagged"},
			createEdits: []codeEdit{appendText(taggedFile, "\nfunc NewTagged() {}\n")},
			renameEdits: []codeEdit{replaceText(taggedFile, "func Tagged()", "func TaggedRenamed()")},
			deleteEdits: []codeEdit{replaceText(taggedFile, "func Tagged() {}\n", "")},
		},
		{
			glyph:       "dirname#DirDiffers",
			create:      &createLeg{"plan:dirname#draft", "func NewDirDiffers()", "plan:dirname#NewDirDiffers"},
			createEdits: []codeEdit{appendText(clauseFile, "\nfunc NewDirDiffers() {}\n")},
			renameEdits: []codeEdit{replaceText(clauseFile, "func DirDiffers()", "func DirDiffersRenamed()")},
			deleteEdits: []codeEdit{replaceText(clauseFile, "func DirDiffers() {}\n", "")},
		},
		{
			glyph:       "cmd/tool#run",
			create:      &createLeg{"plan:cmd/tool#draft", "func newRun()", "plan:cmd/tool#newRun"},
			createEdits: []codeEdit{appendText(mainFile, "\nfunc newRun() {}\n")},
			renameEdits: []codeEdit{replaceText(mainFile, "func run()", "func runRenamed()")},
			deleteEdits: []codeEdit{replaceText(mainFile, "func run() {}\n", "")},
		},
		{
			glyph:       "shapes#BlockVar",
			create:      &createLeg{"plan:shapes#draft", "var NewBlockVar = 6", "plan:shapes#NewBlockVar"},
			createEdits: []codeEdit{replaceText(shapesFile, "\tBlockVar     = 3\n", "\tBlockVar     = 3\n\tNewBlockVar  = 6\n")},
			renameEdits: []codeEdit{replaceText(shapesFile, "\tBlockVar     = 3\n", "\tBlockVarRenamed = 3\n")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "\tBlockVar     = 3\n", "")},
		},
		{
			glyph:       "shapes#NextConst",
			create:      &createLeg{"plan:shapes#draft", "const NewNextConst = 7", "plan:shapes#NewNextConst"},
			createEdits: []codeEdit{replaceText(shapesFile, "\tNextConst\n)", "\tNextConst\n\tNewNextConst = 7\n)")},
			renameEdits: []codeEdit{replaceText(shapesFile, "\tNextConst\n)", "\tNextConstRenamed\n)")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "\tNextConst\n)", ")")},
		},
		{
			glyph:       "shapes#PairB",
			create:      &createLeg{"plan:shapes#draft", "var NewPairB int", "plan:shapes#NewPairB"},
			createEdits: []codeEdit{replaceText(shapesFile, "PairA, PairB = 4, 5\n)", "PairA, PairB = 4, 5\n\tNewPairB int\n)")},
			rename:      &renameLeg{"plan:dirname#PairRenamed", "plan:shapes#PairRenamed"},
			renameEdits: []codeEdit{replaceText(shapesFile, "PairA, PairB = 4, 5", "PairA, PairRenamed = 4, 5")},
			deleteEdits: []codeEdit{replaceText(shapesFile, "PairA, PairB = 4, 5", "PairA = 4")},
		},
	}

	// Fill the fields every row derives from its glyph and legs.
	for i := range rows {
		r := &rows[i]
		r.name = r.glyph
		switch {
		case r.rename != nil:
		case r.glyph == "dirname#DirDiffers":
			r.rename = newRenameLeg(r.glyph, "shapes")
		case r.renameEdits != nil:
			r.rename = newRenameLeg(r.glyph, "dirname")
		}
		if r.deleteEdits != nil && r.deleteGlyphs == nil {
			r.deleteGlyphs = []string{r.glyph}
		}
	}
	return rows
}

// copyGlyphChainFixture copies the committed fixture module into a fresh temporary directory and returns it, so no leg touches the committed tree or another leg's copy.
func copyGlyphChainFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(filepath.Join("testdata", "glyphchain"))); err != nil {
		t.Fatalf("CopyFS(glyphchain fixture) failed: %v", err)
	}
	return dst
}

// writeGlyphPlan writes a valid on-disk plan directory whose card N carries cards[N-1] as its body, with sections written after the overview's Card Index, and returns the directory with the parsed plan.
func writeGlyphPlan(t *testing.T, cards []string, sections ...plankit.Section) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()

	spec := plankit.Plan{Approved: true, Language: "go", Framing: "framing", Sections: sections}
	for i := range cards {
		spec.Cards = append(spec.Cards, plankit.Card{Number: i + 1, Slug: fmt.Sprintf("card%d", i+1), Summary: "summary"})
	}
	files := plankit.Render(spec)
	for i, body := range cards {
		n := i + 1
		files[fmt.Sprintf("%02d-card%d.md", n, n)] = []byte(fmt.Sprintf("# Card %d — card%d\n\n%s\n", n, n, body))
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", name, err)
		}
	}

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	return dir, plan
}

// renameMechanic is the overview section a plan carrying a Rename group must have.
var renameMechanic = plankit.Section{Heading: "Rename mechanic", Body: "Rename through the glyph chain."}

func editCard(targets ...string) string {
	return "**Edit:**\n" + bullets(targets) + "\n**Intent:** edit\n\n**ImpactSummary:** none\n"
}

func deleteCard(targets ...string) string {
	return "**Delete:**\n" + bullets(targets) + "\n**Intent:** delete\n\n**ImpactSummary:** none\n"
}

func createCard(leg *createLeg) string {
	return fmt.Sprintf("**Create:**\n- `%s` -> `%s`\n\n**Intent:** create\n", leg.draft, leg.head)
}

func renameCard(old, draft string) string {
	return fmt.Sprintf("**Rename:**\n- `%s` -> `%s`\n\n**Intent:** rename\n", old, draft)
}

func bullets(targets []string) string {
	var b strings.Builder
	for _, target := range targets {
		fmt.Fprintf(&b, "- `%s`\n", target)
	}
	return b.String()
}

// planGateKeys runs ValidateFormat over the plan in dir against the fixture copy at root and returns its findings as sorted keys.
func planGateKeys(t *testing.T, plan *planparser.Plan, root string) []findingKey {
	t.Helper()
	findings, err := ValidateFormat(plan, root)
	if err != nil {
		t.Fatalf("ValidateFormat(...) returned error: %v", err)
	}
	keys := make([]findingKey, 0, len(findings))
	for _, f := range findings {
		keys = append(keys, findingKey{f.Check, f.Card, f.Severity})
	}
	slices.SortFunc(keys, func(a, b findingKey) int {
		return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})
	return keys
}

// assertPlanGate fails unless ValidateFormat over plan reports exactly want.
func assertPlanGate(t *testing.T, plan *planparser.Plan, root string, want []findingKey) {
	t.Helper()
	got := planGateKeys(t, plan, root)
	if want == nil {
		want = []findingKey{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("ValidateFormat findings = %+v; want %+v", got, want)
	}
}

// TestGlyphChain_PlanGate runs each shape row through its legs against its own fixture copy and plan directory, and asserts the exact finding set of every plan.
func TestGlyphChain_PlanGate(t *testing.T) {
	t.Parallel()

	for _, row := range shapeRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if row.gap != "" {
				t.Logf("known gap: %s", row.gap)
			}

			t.Run("edit", func(t *testing.T) {
				t.Parallel()
				_, plan := writeGlyphPlan(t, []string{editCard(row.glyph)})
				assertPlanGate(t, plan, copyGlyphChainFixture(t), row.editFindings)
			})

			if row.create != nil {
				t.Run("create", func(t *testing.T) {
					t.Parallel()
					root := copyGlyphChainFixture(t)
					dir, plan := writeGlyphPlan(t, []string{createCard(row.create), editCard(row.create.draft)})
					assertPlanGate(t, plan, root, nil)

					for n := 1; n <= 2; n++ {
						got := readCardFile(t, dir, n, fmt.Sprintf("card%d", n))
						if !strings.Contains(got, row.create.canonical) {
							t.Errorf("card %d lacks the canonical handle %q: %s", n, row.create.canonical, got)
						}
					}

					first := []string{readCardFile(t, dir, 1, "card1"), readCardFile(t, dir, 2, "card2")}
					reparsed, err := planparser.ParsePlan(dir)
					if err != nil {
						t.Fatalf("ParsePlan(%q) after canonicalization returned error: %v", dir, err)
					}
					assertPlanGate(t, reparsed, root, nil)
					for n := 1; n <= 2; n++ {
						if got := readCardFile(t, dir, n, fmt.Sprintf("card%d", n)); got != first[n-1] {
							t.Errorf("second ValidateFormat rewrote card %d:\nbefore: %s\nafter: %s", n, first[n-1], got)
						}
					}
				})
			}

			if row.rename != nil {
				t.Run("rename", func(t *testing.T) {
					t.Parallel()
					dir, plan := writeGlyphPlan(t, []string{renameCard(row.glyph, row.rename.draft)}, renameMechanic)
					assertPlanGate(t, plan, copyGlyphChainFixture(t), nil)
					if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, row.rename.canonical) {
						t.Errorf("card 1 lacks the canonical handle %q: %s", row.rename.canonical, got)
					}
				})
			}

			if row.deleteGlyphs != nil {
				t.Run("delete", func(t *testing.T) {
					t.Parallel()
					_, plan := writeGlyphPlan(t, []string{deleteCard(row.deleteGlyphs...)})
					assertPlanGate(t, plan, copyGlyphChainFixture(t), nil)
				})
			}
		})
	}
}

// TestGlyphChain_CrossCard pins the cross-card refusals as they stand: a member and its file, or either and its package self glyph, on two cards, and a Create handle no other card references.
func TestGlyphChain_CrossCard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		cards []string
		want  []findingKey
	}{
		{
			name:  "member on one card and its file on another",
			cards: []string{editCard("shapes#Func"), editCard("shapes/shapes.go")},
			want:  []findingKey{{"containment-file-overlap", "1-card1", SeverityBlocking}},
		},
		{
			name:  "member on one card and its package self glyph on another",
			cards: []string{editCard("shapes#Func"), editCard("shapes#")},
			want:  []findingKey{{"containment-unit-overlap", "1-card1", SeverityBlocking}},
		},
		{
			name:  "file on one card and its package self glyph on another",
			cards: []string{editCard("shapes/shapes.go"), editCard("shapes#")},
			want:  []findingKey{{"containment-unit-overlap", "1-card1", SeverityBlocking}},
		},
		{
			name:  "same file on two cards",
			cards: []string{editCard("shapes/shapes.go"), editCard("shapes/shapes.go")},
		},
		{
			name:  "create handle no other card references",
			cards: []string{createCard(&createLeg{"plan:shapes#draft", "func NewFunc() int", ""})},
			want:  []findingKey{{"handle-unreferenced", "1-card1", SeverityBlocking}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, plan := writeGlyphPlan(t, tc.cards)
			assertPlanGate(t, plan, copyGlyphChainFixture(t), tc.want)
		})
	}
}
