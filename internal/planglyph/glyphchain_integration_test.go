//go:build integration

// glyphchain_integration_test.go carries each glyph-chain shape row's legs past the plan gate through a real commit's delta, handle binding, the on-disk rewrite and the done-checks, spawning git through Delta — the reason this file carries the integration build tag.
// glyphchain_test.go holds the shape table and the untagged plan-gate legs.

package planglyph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"golang.org/x/tools/go/packages"
)

// newGlyphChainRepo returns a delta fixture repository holding a copy of the committed fixture module, and the SHA of the commit that adds it.
func newGlyphChainRepo(t *testing.T) (*deltaFixtureRepo, string) {
	t.Helper()
	f := newDeltaFixtureRepo(t)
	if err := os.CopyFS(f.root, os.DirFS(filepath.Join("testdata", "glyphchain"))); err != nil {
		t.Fatalf("CopyFS(glyphchain fixture) failed: %v", err)
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", "base")
	return f, f.git("rev-parse", "HEAD")
}

// commitEdits applies edits to the fixture repository's files, commits them and returns the commit's SHA.
// Each non-empty old must occur exactly once in its file, so a stale edit fails loudly instead of silently changing nothing.
func (f *deltaFixtureRepo) commitEdits(edits []codeEdit) string {
	f.t.Helper()
	for _, e := range edits {
		full := filepath.Join(f.root, filepath.FromSlash(e.path))
		data, err := os.ReadFile(full)
		if err != nil {
			f.t.Fatalf("read %q: %v", full, err)
		}
		text := string(data)
		if e.old == "" {
			text += e.new
		} else {
			if n := strings.Count(text, e.old); n != 1 {
				f.t.Fatalf("edit of %s: %q occurs %d times; want exactly once", e.path, e.old, n)
			}
			text = strings.Replace(text, e.old, e.new, 1)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			f.t.Fatalf("write %q: %v", full, err)
		}
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", "edit")
	return f.git("rev-parse", "HEAD")
}

// bindCardOne binds card 1's handles of the plan in dir from the delta between base and head, requiring no finding, and returns the plan re-read from disk.
func bindCardOne(t *testing.T, f *deltaFixtureRepo, dir, base, head string) *planparser.Plan {
	t.Helper()
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	delta, err := Delta(f.root, base, head)
	if err != nil {
		t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, base, head, err)
	}
	findings, err := BindHandles(plan, dir, delta, plan.Cards[:1])
	if err != nil {
		t.Fatalf("BindHandles(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("BindHandles(...) findings = %+v; want none", findings)
	}
	bound, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) after binding returned error: %v", dir, err)
	}
	return bound
}

// requireDoneChecksClean fails unless DoneChecks over cards reports nothing against the fixture repository's current tree.
func requireDoneChecksClean(t *testing.T, plan *planparser.Plan, cards []planparser.Card, root string) {
	t.Helper()
	findings, err := DoneChecks(plan, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("DoneChecks(...) findings = %+v; want none", findings)
	}
}

// TestGlyphChain_Delta runs each shape row's Create, Rename and Delete legs, each in its own repository, past the plan gate through a committed code change, the delta, handle binding and the done-checks.
func TestGlyphChain_Delta(t *testing.T) {
	t.Parallel()

	for _, row := range shapeRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			if row.create != nil {
				t.Run("create", func(t *testing.T) {
					t.Parallel()
					f, base := newGlyphChainRepo(t)
					dir, plan := writeGlyphPlan(t, []string{createCard(row.create), editCard(row.create.draft)})
					assertPlanGate(t, plan, f.root, nil)

					bound := bindCardOne(t, f, dir, base, f.commitEdits(row.createEdits))

					glyph := strings.TrimPrefix(row.create.canonical, "plan:")
					if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "- `"+glyph+"`\n") || strings.Contains(got, "->") {
						t.Errorf("card 1 = %q; want its declaration bullet collapsed to `%s`", got, glyph)
					}
					if got := readCardFile(t, dir, 2, "card2"); !strings.Contains(got, "`"+glyph+"`") || strings.Contains(got, planparser.HandlePrefix) {
						t.Errorf("card 2 = %q; want the bound glyph %q and no handle", got, glyph)
					}
					requireDoneChecksClean(t, bound, bound.Cards[:1], f.root)
				})
			}

			if row.rename != nil {
				t.Run("rename", func(t *testing.T) {
					t.Parallel()
					f, base := newGlyphChainRepo(t)
					dir, plan := writeGlyphPlan(t, []string{renameCard(row.glyph, row.rename.draft)}, renameMechanic)
					assertPlanGate(t, plan, f.root, nil)

					bound := bindCardOne(t, f, dir, base, f.commitEdits(row.renameEdits))

					want := fmt.Sprintf("`%s` -> `%s`", row.glyph, strings.TrimPrefix(row.rename.canonical, "plan:"))
					if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, want) {
						t.Errorf("card 1 = %q; want the pair bound to %s", got, want)
					}
					requireDoneChecksClean(t, bound, bound.Cards[:1], f.root)
				})
			}

			if row.deleteGlyphs != nil {
				t.Run("delete", func(t *testing.T) {
					t.Parallel()
					f, _ := newGlyphChainRepo(t)
					_, plan := writeGlyphPlan(t, []string{deleteCard(row.deleteGlyphs...)})

					f.commitEdits(row.deleteEdits)

					requireDoneChecksClean(t, plan, plan.Cards, f.root)
				})
			}
		})
	}
}

// TestGlyphChain_UnreferencedHandle pins that a Create handle no other card references is bound at record-batch and passes its done-check.
func TestGlyphChain_UnreferencedHandle(t *testing.T) {
	t.Parallel()

	f, base := newGlyphChainRepo(t)
	dir, plan := writeGlyphPlan(t, []string{createCard(&createLeg{"plan:shapes#draft", "func NewFunc() int", ""})})
	// The plan gate's own verdict is TestGlyphChain_CrossCard's; here it only canonicalizes the draft handle the bind matches.
	if _, err := ValidateFormat(plan, f.root); err != nil {
		t.Fatalf("ValidateFormat(...) returned error: %v", err)
	}

	bound := bindCardOne(t, f, dir, base, f.commitEdits([]codeEdit{appendText(shapesFile, "\nfunc NewFunc() int { return 1 }\n")}))

	if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "- `shapes#NewFunc`\n") {
		t.Errorf("card 1 = %q; want its declaration bullet collapsed to `shapes#NewFunc`", got)
	}
	requireDoneChecksClean(t, bound, bound.Cards[:1], f.root)
}

// TestGlyphChain_ReworkGenerations pins the rework-generations scenario: three cards share a file and validate clean at the plan gate, and once card 1's code is bound a dispatch with card 1 completed reports no blocking finding and no redundant-file-target for card 2's file and handle built in that file.
func TestGlyphChain_ReworkGenerations(t *testing.T) {
	t.Parallel()

	f, base := newGlyphChainRepo(t)
	dir, plan := writeGlyphPlan(t, []string{
		"**Create:**\n- `plan:shapes#Told` -> `type Told struct{}`\n\n**Edit:**\n- `shapes/shapes.go`\n\n**Intent:** one\n\n**ImpactSummary:** none\n",
		editCard("shapes/shapes.go", "plan:shapes#Told"),
		editCard("shapes/shapes.go"),
	})
	assertPlanGate(t, plan, f.root, nil)

	bound := bindCardOne(t, f, dir, base, f.commitEdits([]codeEdit{appendText(shapesFile, "\ntype Told struct{}\n")}))

	findings, err := ValidateDispatch(bound, f.root, bound.Cards[:1], nil)
	if err != nil {
		t.Fatalf("ValidateDispatch(...) returned error: %v", err)
	}
	for _, finding := range findings {
		if finding.Check == "redundant-file-target" {
			t.Errorf("ValidateDispatch reported %+v; want the plan-gate pass skipped", finding)
		}
		if finding.Severity == SeverityBlocking {
			t.Errorf("ValidateDispatch reported blocking finding %+v; want none", finding)
		}
	}
}

// TestGlyphChain_RedundantPackage pins redundant-package-target across the plan gate and dispatch: a package self glyph beside a handle of that package is flagged both before and after card 1's handle is bound, and a rename's to-side handle belongs to the old glyph's package, not the package its draft spelling names.
func TestGlyphChain_RedundantPackage(t *testing.T) {
	t.Parallel()

	redundant := []findingKey{{"redundant-package-target", "2-card2", SeverityBlocking}}

	tests := []struct {
		name      string
		cards     []string
		sections  []plankit.Section
		edits     []codeEdit
		wantGate  []findingKey
		wantBound []findingKey
	}{
		{
			name:      "create handle beside its package self glyph",
			cards:     []string{createCard(&createLeg{"plan:shapes#Fresh", "func Fresh() int", ""}), editCard("shapes#", "plan:shapes#Fresh")},
			edits:     []codeEdit{appendText(shapesFile, "\nfunc Fresh() int { return 1 }\n")},
			wantGate:  redundant,
			wantBound: redundant,
		},
		{
			name:     "rename to-side handle beside the draft unit's package self glyph",
			cards:    []string{renameCard("shapes#Func", "plan:dirname#FuncMoved"), editCard("dirname#", "plan:dirname#FuncMoved")},
			sections: []plankit.Section{renameMechanic},
			edits:    []codeEdit{replaceText(shapesFile, "func Func() int", "func FuncMoved() int")},
		},
		{
			name:      "rename to-side handle beside the old unit's package self glyph",
			cards:     []string{renameCard("shapes#Func", "plan:dirname#FuncMoved"), editCard("shapes#", "plan:dirname#FuncMoved")},
			sections:  []plankit.Section{renameMechanic},
			edits:     []codeEdit{replaceText(shapesFile, "func Func() int", "func FuncMoved() int")},
			wantGate:  redundant,
			wantBound: redundant,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, base := newGlyphChainRepo(t)
			dir, plan := writeGlyphPlan(t, tc.cards, tc.sections...)
			assertPlanGate(t, plan, f.root, tc.wantGate)

			bound := bindCardOne(t, f, dir, base, f.commitEdits(tc.edits))

			findings, err := ValidateDispatch(bound, f.root, bound.Cards[:1], nil)
			if err != nil {
				t.Fatalf("ValidateDispatch(...) returned error: %v", err)
			}
			var got []findingKey
			for _, finding := range findings {
				if finding.Severity == SeverityBlocking {
					got = append(got, findingKey{finding.Check, finding.Card, finding.Severity})
				}
			}
			if !slices.Equal(got, tc.wantBound) {
				t.Errorf("ValidateDispatch blocking findings = %+v; want %+v", got, tc.wantBound)
			}
		})
	}
}

// fatalTypesLoader fails the test when planGatePass calls it.
type fatalTypesLoader struct{ t *testing.T }

func (l fatalTypesLoader) load(string, []string) ([]*packages.Package, error) {
	l.t.Error("typesLoader called; want the load skipped")
	return nil, errors.New("unexpected load")
}

// TestGlyphChain_CallerCoverageTyped pins caller-uncovered with a real go list load: type information tells receivers apart, and the import-path scan answers where the load has none.
func TestGlyphChain_CallerCoverageTyped(t *testing.T) {
	t.Parallel()

	const (
		methodDecl  = "package callers\n\ntype Other struct{}\n\nfunc (Other) Method() int { return 0 }\n\nfunc useOther(u Other) int { return u.Method() }\n"
		methodCover = "callers#UseMethod"
	)
	covers := []string{"callees/local.go", "callees/callees_external_test.go"}
	threeLines := "package broken\n\nimport \"example.com/glyphchain/callees\"\n\ntype other struct{}\n\nfunc (other) Method() int { return 0 }\n\n" +
		"func f(t callees.Thing, u other) {\n" +
		"\tundefined().Method()\n" +
		"\tundefined().Method(); _ = t.Method()\n" +
		"\tu.Method()\n" +
		"}\n"

	cases := []struct {
		name  string
		cards []string
		files map[string]string
		// dropRootModule removes the fixture's root go.mod, which skips the load.
		dropRootModule bool
		// want lists "<severity> <file>" for every caller-uncovered finding, sorted.
		want []string
		// wantDetails are substrings some caller-uncovered finding's detail holds.
		wantDetails []string
	}{
		{
			name:  "a same-named method on another type is not a reference",
			cards: []string{deleteCard("callees#Thing.Method")},
			files: map[string]string{"callers/other.go": methodDecl},
			want:  []string{"blocking callers/callers.go"},
		},
		{
			name:  "a package-qualified function of the same name is not a reference",
			cards: []string{deleteCard("callees#Thing.Method")},
			files: map[string]string{
				"other/method.go":     "package other\n\nfunc Method() int { return 0 }\n",
				"callers/useother.go": "package callers\n\nimport \"example.com/glyphchain/other\"\n\nfunc useOther() int { return other.Method() }\n",
			},
			want: []string{"blocking callers/callers.go"},
		},
		{
			name:  "a promoted call and a method value are blocking",
			cards: []string{deleteWithEdit([]string{"callees#Thing.Method"}, []string{methodCover})},
			files: map[string]string{
				"callers/promoted.go": "package callers\n\nimport \"example.com/glyphchain/callees\"\n\ntype Embeds struct{ callees.Thing }\n\nfunc promoted(e Embeds) int { return e.Method() }\n",
				"callers/value.go":    "package callers\n\nimport \"example.com/glyphchain/callees\"\n\nfunc value(t callees.Thing) func() int { return t.Method }\n",
			},
			want: []string{"blocking callers/promoted.go", "blocking callers/value.go"},
		},
		{
			name:  "a call through an interface holding the method is not a reference",
			cards: []string{deleteWithEdit([]string{"callees#Thing.Method"}, []string{methodCover})},
			files: map[string]string{
				"callers/iface.go": "package callers\n\ntype Methoder interface{ Method() int }\n\nfunc viaInterface(m Methoder) int { return m.Method() }\n",
			},
		},
		{
			name:  "an aliased importer is blocking through types",
			cards: []string{coveredTargetCard(covers)},
			files: map[string]string{"callers/aliased.go": aliasedImporter},
			want:  []string{"blocking callers/aliased.go"},
		},
		{
			name:  "a dot importer is blocking through types",
			cards: []string{coveredTargetCard(covers)},
			files: map[string]string{"dotted/dotted.go": dotImporter},
			want:  []string{"blocking dotted/dotted.go"},
		},
		{
			name:  "a file for another GOOS is answered by the scan",
			cards: []string{deleteWithEdit([]string{"callees#Target", "callees#Thing.Method"}, append(slices.Clone(covers), "callers#"))},
			files: map[string]string{
				"tagged/plan9.go": "//go:build plan9\n\npackage tagged\n\nimport \"example.com/glyphchain/callees\"\n\nfunc plan9(t callees.Thing) int { callees.Target(); return t.Method() }\n",
			},
			want:        []string{"blocking tagged/plan9.go", "informational tagged/plan9.go"},
			wantDetails: []string{"the receiver could not be resolved"},
		},
		{
			name:  "a nested module leaves the typed verdicts of the root files intact",
			cards: []string{deleteCard("callees#Thing.Method")},
			files: map[string]string{
				"callers/other.go": methodDecl,
				"nested/go.mod":    nestedModuleFile,
				"nested/use.go":    "package nested\n\nimport \"example.com/glyphchain/callees\"\n\nfunc use(t callees.Thing) int { return t.Method() }\n",
			},
			want: []string{"blocking callers/callers.go", "informational nested/use.go"},
		},
		{
			name:  "an identifier without type information goes to the scan on its own line",
			cards: []string{deleteWithEdit([]string{"callees#Thing.Method"}, []string{methodCover})},
			files: map[string]string{"broken/broken.go": threeLines},
			want:  []string{"blocking broken/broken.go", "informational broken/broken.go"},
			// Lines 10 and 11 hold the undefined receivers; line 11 also holds the typed call.
			wantDetails: []string{"broken/broken.go at line 10 ", "broken/broken.go references it at line 11 "},
		},
		{
			name:  "a file that does not parse is answered by the scan",
			cards: []string{deleteCard("callees#Target")},
			// The parser drops everything after the bad operand, so the call on line 7 is missing from the file's syntax.
			files: map[string]string{"broken/broken.go": "package broken\n\nimport \"example.com/glyphchain/callees\"\n\nfunc f() {\n\tx := )\n\tcallees.Target()\n}\n"},
			want:  []string{"blocking broken/broken.go", "blocking callees/callees_external_test.go", "blocking callees/local.go", "blocking callers/callers.go"},
		},
		{
			name:           "a root without go.mod skips the load and is answered by the scan",
			cards:          []string{deleteCard("callees#Target")},
			dropRootModule: true,
			want:           []string{"blocking callees/callees_external_test.go", "blocking callees/local.go", "blocking callers/callers.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := copyGlyphChainFixture(t)
			writeFixtureFiles(t, root, tc.files)
			var loader typesLoader = goListLoader{timeout: time.Minute}
			if tc.dropRootModule {
				if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
					t.Fatalf("Remove(go.mod) failed: %v", err)
				}
				loader = fatalTypesLoader{t}
			}
			_, plan := writeGlyphPlan(t, tc.cards)

			findings, err := planGatePass(plan, root, loader)
			if err != nil {
				t.Fatalf("planGatePass(...) returned error: %v", err)
			}
			var got, details []string
			for _, f := range findings {
				if f.Check == "caller-uncovered" {
					got = append(got, fmt.Sprintf("%s %s", f.Severity, coverageFileOf(f.Detail)))
					details = append(details, f.Detail)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("caller-uncovered findings = %q; want %q", got, tc.want)
			}
			for _, want := range tc.wantDetails {
				if !slices.ContainsFunc(details, func(detail string) bool { return strings.Contains(detail, want) }) {
					t.Errorf("no caller-uncovered detail holds %q; details = %q", want, details)
				}
			}
		})
	}

	t.Run("a slotted load waits for its slot and answers as the unslotted load", func(t *testing.T) {
		t.Parallel()
		root := copyGlyphChainFixture(t)
		_, plan := writeGlyphPlan(t, []string{deleteCard("callees#Target")})

		want, err := planGatePass(plan, root, goListLoader{timeout: time.Minute})
		if err != nil {
			t.Fatalf("unslotted planGatePass(...) returned error: %v", err)
		}

		pool := &gateslot.Pool{
			Dir:    t.TempDir(),
			Limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1, GoParallel: 1}, nil },
			Poll:   5 * time.Millisecond,
		}
		held, err := pool.Acquire(context.Background(), gateslot.Holder{Worktree: root, Site: "test holder"})
		if err != nil {
			t.Fatalf("Acquire(...) returned error: %v", err)
		}
		releaseHeld := sync.OnceFunc(func() {
			if err := held.Release(); err != nil {
				t.Errorf("Release() returned error: %v", err)
			}
		})
		defer releaseHeld()

		waitDir := t.TempDir()
		type outcome struct {
			findings []Finding
			err      error
		}
		done := make(chan outcome, 1)
		go func() {
			findings, err := planGatePass(plan, root, goListLoader{timeout: time.Minute, slots: pool, waitDir: waitDir})
			done <- outcome{findings, err}
		}()

		var waits []gateslot.Wait
		for deadline := time.Now().Add(30 * time.Second); len(waits) == 0; {
			if time.Now().After(deadline) {
				t.Fatal("no wait record appeared while the slot was held")
			}
			if waits, err = gateslot.ReadWaits(waitDir); err != nil {
				t.Fatalf("ReadWaits(...) returned error: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
		if waits[0].Site != typesLoadSite {
			t.Errorf("wait record site = %q; want %q", waits[0].Site, typesLoadSite)
		}
		select {
		case got := <-done:
			t.Fatalf("the load returned %+v while the only slot was held", got)
		default:
		}

		releaseHeld()
		got := <-done
		if got.err != nil {
			t.Fatalf("slotted planGatePass(...) returned error: %v", got.err)
		}
		if !reflect.DeepEqual(got.findings, want) {
			t.Errorf("slotted findings = %+v; want the unslotted %+v", got.findings, want)
		}
		if waits, err = gateslot.ReadWaits(waitDir); err != nil || len(waits) != 0 {
			t.Errorf("ReadWaits after the load = (%+v, %v); want no records", waits, err)
		}
	})
}
