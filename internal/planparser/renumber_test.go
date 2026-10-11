// renumber_test.go pins InsertCard and RemoveCard: the renumbered plan, the refusals that leave the plan directory untouched, and the reversal of an apply that fails at each step.

package planparser_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

const (
	renumberJournalName = "renumber-journal.json"
	renumberOverview    = "00-overview.md"
)

// renumberFixture is a plan of four cards plus archived generations, and the journal path whose parent directory does not exist yet.
type renumberFixture struct {
	root        string
	planDir     string
	journalPath string
	first       int
}

// newRenumberFixture writes cards alpha, beta, gamma and delta numbered from first, with card references in the framing, a plan section and gamma's intent.
func newRenumberFixture(t *testing.T, first int) renumberFixture {
	t.Helper()
	root := t.TempDir()
	planDir := filepath.Join(root, "plan")

	plan := plankit.Plan{
		Framing: fmt.Sprintf("Covers cards %d–%d.", first+1, first+2),
		Sections: []plankit.Section{
			{Heading: "Shared Decisions", Body: fmt.Sprintf("- **Applies to:** cards %d, %d", first+1, first+2)},
		},
		Cards: []plankit.Card{
			{Number: first, Slug: "alpha", Summary: "intent of alpha", Intent: fmt.Sprintf("Stands alone; see card %d.", first)},
			{Number: first + 1, Slug: "beta", Summary: "intent of beta", Commit: fmt.Sprintf("%d: beta", first+1)},
			{Number: first + 2, Slug: "gamma", Summary: "intent of gamma", Intent: gammaIntent(first)},
			{Number: first + 3, Slug: "delta", Summary: "intent of delta"},
		},
	}
	if first != 1 {
		plan.FirstCard = first
	}
	plankit.Write(t, planDir, plan)
	plankit.WriteTree(t, planDir, map[string]string{
		"archive-1/00-overview.md":                "archived overview\n",
		"archive-1/01-old.md":                     "# Card 1 — old\n",
		"round-1/prior-generation/02-prior.md":    "# Card 2 — prior\n",
		"round-1/prior-generation/00-overview.md": "prior overview\n",
	})

	return renumberFixture{root: root, planDir: planDir, journalPath: filepath.Join(root, "scratch", "sub", renumberJournalName), first: first}
}

func gammaIntent(first int) string {
	return fmt.Sprintf("Builds on card %d and wraps cards %d, %d.", first+1, first+2, first+3)
}

// snapshot returns the content of every file under root, keyed by slash-separated relative path.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot of %s failed: %v", root, err)
	}
	return files
}

// requireSameTree fails when after differs from before in any file's presence or bytes.
func requireSameTree(t *testing.T, before, after map[string]string) {
	t.Helper()
	if maps.Equal(before, after) {
		return
	}
	var diff []string
	for name, content := range before {
		if got, ok := after[name]; !ok {
			diff = append(diff, "missing: "+name)
		} else if got != content {
			diff = append(diff, "changed: "+name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			diff = append(diff, "extra: "+name)
		}
	}
	sort.Strings(diff)
	t.Fatalf("tree differs from before: %v", diff)
}

// fault fails the next times calls of one RenumberIO operation on the file with the given base name.
type fault struct {
	op    string
	base  string
	times int
}

// faultyIO returns a RenumberIO over the real file system that fails according to faults.
func faultyIO(faults ...*fault) planparser.RenumberIO {
	ops := planparser.OSRenumberIO()
	injected := func(op, name string) error {
		for _, f := range faults {
			if f.op == op && f.base == filepath.Base(name) && f.times > 0 {
				f.times--
				return fmt.Errorf("injected %s failure on %s", op, f.base)
			}
		}
		return nil
	}
	ops.ReadFile = func(name string) ([]byte, error) {
		if err := injected("ReadFile", name); err != nil {
			return nil, err
		}
		return os.ReadFile(name)
	}
	ops.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		if err := injected("WriteFile", name); err != nil {
			return err
		}
		return os.WriteFile(name, data, perm)
	}
	ops.Rename = func(oldpath, newpath string) error {
		if err := injected("Rename", oldpath); err != nil {
			return err
		}
		return os.Rename(oldpath, newpath)
	}
	ops.Remove = func(name string) error {
		if err := injected("Remove", name); err != nil {
			return err
		}
		return os.Remove(name)
	}
	ops.MkdirAll = func(path string, perm os.FileMode) error {
		if err := injected("MkdirAll", path); err != nil {
			return err
		}
		return os.MkdirAll(path, perm)
	}
	return ops
}

// renumberCall runs one verb against a fixture with the given file seam.
type renumberCall func(ops planparser.RenumberIO, fx renumberFixture) (*planparser.RenumberResult, error)

func insertAt(offset int) renumberCall {
	return func(ops planparser.RenumberIO, fx renumberFixture) (*planparser.RenumberResult, error) {
		return planparser.InsertCard(ops, fx.planDir, fx.journalPath, fx.first+offset, "new", "a new card")
	}
}

func removeAt(offset int) renumberCall {
	return func(ops planparser.RenumberIO, fx renumberFixture) (*planparser.RenumberResult, error) {
		return planparser.RemoveCard(ops, fx.planDir, fx.journalPath, fx.first+offset)
	}
}

func TestRenumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		first     int
		strayTemp bool
		call      func(offset int) renumberCall
		offset    int
		wantSlugs []string
		// wantMoved is each move's old and new offset from the first card, in apply order.
		wantMoved [][2]int
		// wantMentions reports whether the framing, the plan section and gamma's intent name a moved card by its old number.
		wantMentions bool
	}{
		{name: "insert at a middle position", first: 1, call: insertAt, offset: 1, wantSlugs: []string{"alpha", "new", "beta", "gamma", "delta"}, wantMoved: [][2]int{{3, 4}, {2, 3}, {1, 2}}, wantMentions: true},
		{name: "insert one past the last card", first: 1, call: insertAt, offset: 4, wantSlugs: []string{"alpha", "beta", "gamma", "delta", "new"}},
		{name: "insert ignores a stray temporary journal", first: 1, strayTemp: true, call: insertAt, offset: 4, wantSlugs: []string{"alpha", "beta", "gamma", "delta", "new"}},
		{name: "remove at a middle position", first: 1, call: removeAt, offset: 1, wantSlugs: []string{"alpha", "gamma", "delta"}, wantMoved: [][2]int{{2, 1}, {3, 2}}, wantMentions: true},
		{name: "remove the last card", first: 1, call: removeAt, offset: 3, wantSlugs: []string{"alpha", "beta", "gamma"}},
		{name: "insert on a plan starting above 1", first: 5, call: insertAt, offset: 1, wantSlugs: []string{"alpha", "new", "beta", "gamma", "delta"}, wantMoved: [][2]int{{3, 4}, {2, 3}, {1, 2}}, wantMentions: true},
		{name: "remove on a plan starting above 1", first: 5, call: removeAt, offset: 1, wantSlugs: []string{"alpha", "gamma", "delta"}, wantMoved: [][2]int{{2, 1}, {3, 2}}, wantMentions: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRenumberFixture(t, tc.first)
			if tc.strayTemp {
				plankit.WriteTree(t, filepath.Dir(fx.journalPath), map[string]string{renumberJournalName + ".tmp": "stray"})
			}
			before := snapshot(t, fx.root)
			beforePlan, err := planparser.ParsePlan(fx.planDir)
			if err != nil {
				t.Fatalf("ParsePlan before: %v", err)
			}

			result, err := tc.call(tc.offset)(planparser.OSRenumberIO(), fx)
			if err != nil {
				t.Fatalf("renumber failed: %v", err)
			}
			after := snapshot(t, fx.root)

			plan, err := planparser.ParsePlan(fx.planDir)
			if err != nil {
				t.Fatalf("ParsePlan after: %v", err)
			}
			var gotSlugs []string
			for i, c := range plan.Cards {
				gotSlugs = append(gotSlugs, c.Slug)
				if c.Number != tc.first+i {
					t.Errorf("card %q numbered %d; want %d", c.Slug, c.Number, tc.first+i)
				}
			}
			if !slices.Equal(gotSlugs, tc.wantSlugs) {
				t.Fatalf("card order = %v; want %v", gotSlugs, tc.wantSlugs)
			}
			for _, f := range planparser.ValidateFormat(plan, fx.root) {
				if f.Check == "index-file-mismatch" || f.Check == "card-numbering" {
					t.Errorf("ValidateFormat reported %v", f)
				}
			}

			wantSummaries := map[string]string{"new": "a new card"}
			for _, c := range beforePlan.Cards {
				wantSummaries[c.Slug] = c.Summary
			}
			for _, c := range plan.Cards {
				if c.Summary != wantSummaries[c.Slug] {
					t.Errorf("card %q Card Index text = %q; want %q", c.Slug, c.Summary, wantSummaries[c.Slug])
				}
			}

			var gotMoved [][2]int
			for _, m := range result.Moved {
				gotMoved = append(gotMoved, [2]int{m.OldNumber - tc.first, m.NewNumber - tc.first})
				oldContent := before["plan/"+m.OldFile]
				oldNumber, newNumber := fmt.Sprint(m.OldNumber), fmt.Sprint(m.NewNumber)
				want := strings.NewReplacer("# Card "+oldNumber+" —", "# Card "+newNumber+" —", "`"+oldNumber+": ", "`"+newNumber+": ").Replace(oldContent)
				if after["plan/"+m.NewFile] != want {
					t.Errorf("moved card %s = %q; want %q", m.NewFile, after["plan/"+m.NewFile], want)
				}
				if _, ok := after["plan/"+m.OldFile]; ok && m.OldFile != m.NewFile {
					t.Errorf("old file %s still exists beside the moved card", m.OldFile)
				}
			}
			if !slices.Equal(gotMoved, tc.wantMoved) {
				t.Errorf("Moved = %v; want %v", gotMoved, tc.wantMoved)
			}

			for name, content := range before {
				if strings.HasPrefix(name, "plan/archive-1/") || strings.HasPrefix(name, "plan/round-1/") {
					if after[name] != content {
						t.Errorf("%s changed", name)
					}
				}
			}
			framedBefore, err := planparser.OverviewWithoutCardIndex([]byte(before["plan/"+renumberOverview]))
			if err != nil {
				t.Fatal(err)
			}
			framedAfter, err := planparser.OverviewWithoutCardIndex([]byte(after["plan/"+renumberOverview]))
			if err != nil {
				t.Fatal(err)
			}
			if string(framedBefore) != string(framedAfter) {
				t.Errorf("overview changed outside the Card Index")
			}

			if _, ok := after["scratch/sub/"+renumberJournalName]; ok {
				t.Errorf("journal left on disk")
			}
			if _, ok := after["scratch/sub/"+renumberJournalName+".tmp"]; ok {
				t.Errorf("temporary journal left on disk")
			}

			for _, created := range result.Created {
				stub := fmt.Sprintf("# Card %d — new\n", tc.first+tc.offset)
				if got := after["plan/"+created]; got != stub {
					t.Errorf("new card file %s = %q; want %q", created, got, stub)
				}
			}

			var wantMentions []string
			if tc.wantMentions {
				gammaFile := fmt.Sprintf("%02d-gamma.md", tc.first+slices.Index(tc.wantSlugs, "gamma"))
				wantMentions = []string{
					renumberOverview + ": " + fmt.Sprintf("Covers cards %d–%d.", tc.first+1, tc.first+2),
					renumberOverview + ": " + fmt.Sprintf("- **Applies to:** cards %d, %d", tc.first+1, tc.first+2),
					gammaFile + ": **Intent:** " + gammaIntent(tc.first),
				}
			}
			var gotMentions []string
			for _, p := range result.ProseMentions {
				gotMentions = append(gotMentions, p.File+": "+p.Text)
				lines := strings.Split(after["plan/"+p.File], "\n")
				if p.Line < 1 || p.Line > len(lines) || strings.TrimSpace(lines[p.Line-1]) != p.Text {
					t.Errorf("ProseMention %+v does not point at its line", p)
				}
			}
			if !slices.Equal(gotMentions, wantMentions) {
				t.Errorf("ProseMentions = %q; want %q", gotMentions, wantMentions)
			}
		})
	}
}

func TestRenumber_Refusals(t *testing.T) {
	t.Parallel()

	verbs := map[string]func(ops planparser.RenumberIO, fx renumberFixture, position int, slug, intent string) error{
		"insert": func(ops planparser.RenumberIO, fx renumberFixture, position int, slug, intent string) error {
			_, err := planparser.InsertCard(ops, fx.planDir, fx.journalPath, position, slug, intent)
			return err
		},
		"remove": func(ops planparser.RenumberIO, fx renumberFixture, position int, _, _ string) error {
			_, err := planparser.RemoveCard(ops, fx.planDir, fx.journalPath, position)
			return err
		},
	}

	tests := []struct {
		name  string
		verbs []string
		// prepare changes the fixture before the call and may return a fault-injecting seam.
		prepare  func(t *testing.T, fx renumberFixture) planparser.RenumberIO
		position int
		slug     string
		intent   string
		want     error
		wantText string
	}{
		{name: "journal present", verbs: []string{"insert", "remove"}, position: 2, slug: "new", intent: "x", want: planparser.ErrRenumberJournalExists,
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				plankit.WriteTree(t, filepath.Dir(fx.journalPath), map[string]string{renumberJournalName: "{}"})
				return planparser.OSRenumberIO()
			}},
		{name: "journal unreadable", verbs: []string{"insert", "remove"}, position: 2, slug: "new", intent: "x", want: planparser.ErrRenumberIOFailed,
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				return faultyIO(&fault{op: "ReadFile", base: renumberJournalName, times: 1})
			}},
		{name: "overview missing", verbs: []string{"insert", "remove"}, position: 2, slug: "new", intent: "x", wantText: "plan overview not found",
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				if err := os.Remove(filepath.Join(fx.planDir, renumberOverview)); err != nil {
					t.Fatal(err)
				}
				return planparser.OSRenumberIO()
			}},
		{name: "card file not in the Card Index", verbs: []string{"insert", "remove"}, position: 2, slug: "new", intent: "x", want: planparser.ErrRenumberPlanInconsistent,
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				plankit.WriteTree(t, fx.planDir, map[string]string{"09-orphan.md": "# Card 9 — orphan\n"})
				return planparser.OSRenumberIO()
			}},
		{name: "heading number differs from the Card Index", verbs: []string{"insert", "remove"}, position: 2, slug: "new", intent: "x", want: planparser.ErrRenumberPlanInconsistent,
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				plankit.WriteTree(t, fx.planDir, map[string]string{"02-beta.md": "# Card 7 — beta\n"})
				return planparser.OSRenumberIO()
			}},
		{name: "insert below the first card", verbs: []string{"insert"}, position: 0, slug: "new", intent: "x", want: planparser.ErrRenumberInput},
		{name: "insert more than one past the last card", verbs: []string{"insert"}, position: 6, slug: "new", intent: "x", want: planparser.ErrRenumberInput},
		{name: "remove below the first card", verbs: []string{"remove"}, position: 0, want: planparser.ErrRenumberInput},
		{name: "remove past the last card", verbs: []string{"remove"}, position: 5, want: planparser.ErrRenumberInput},
		{name: "slug with a path separator", verbs: []string{"insert"}, position: 2, slug: "a/b", intent: "x", want: planparser.ErrRenumberInput},
		{name: "slug escaping the directory", verbs: []string{"insert"}, position: 2, slug: "../x", intent: "x", want: planparser.ErrRenumberInput},
		{name: "slug with a double hyphen", verbs: []string{"insert"}, position: 2, slug: "a--b", intent: "x", want: planparser.ErrRenumberInput},
		{name: "empty slug", verbs: []string{"insert"}, position: 2, slug: "", intent: "x", want: planparser.ErrRenumberInput},
		{name: "blank intent", verbs: []string{"insert"}, position: 2, slug: "new", intent: " \t ", want: planparser.ErrRenumberInput},
		{name: "intent with a line break", verbs: []string{"insert"}, position: 2, slug: "new", intent: "one\ntwo", want: planparser.ErrRenumberInput},
		{name: "remove the only card", verbs: []string{"remove"}, position: 1, want: planparser.ErrRenumberInput,
			prepare: func(t *testing.T, fx renumberFixture) planparser.RenumberIO {
				plankit.Write(t, fx.planDir, plankit.Plan{Cards: []plankit.Card{{Number: 1, Slug: "only"}}})
				for _, name := range []string{"01-alpha.md", "02-beta.md", "03-gamma.md", "04-delta.md"} {
					if err := os.Remove(filepath.Join(fx.planDir, name)); err != nil {
						t.Fatal(err)
					}
				}
				return planparser.OSRenumberIO()
			}},
	}
	for _, tc := range tests {
		for _, verb := range tc.verbs {
			t.Run(tc.name+"/"+verb, func(t *testing.T) {
				t.Parallel()
				fx := newRenumberFixture(t, 1)
				ops := planparser.OSRenumberIO()
				if tc.prepare != nil {
					ops = tc.prepare(t, fx)
				}
				before := snapshot(t, fx.root)

				err := verbs[verb](ops, fx, tc.position, tc.slug, tc.intent)
				if err == nil {
					t.Fatalf("renumber succeeded; want a refusal")
				}
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Errorf("error = %v; want it to wrap %v", err, tc.want)
				}
				if !strings.Contains(err.Error(), tc.wantText) {
					t.Errorf("error = %q; want it to contain %q", err, tc.wantText)
				}
				requireSameTree(t, before, snapshot(t, fx.root))
			})
		}
	}
}

func TestRenumber_IOFailureReversed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		call  renumberCall
		fault fault
		// want is the sentinel the failure ends in; a ErrRenumberReversalFailed row keeps the journal.
		want error
	}{
		{name: "insert: journal directory creation", call: insertAt(1), fault: fault{op: "MkdirAll", base: "sub", times: 1}},
		{name: "insert: temporary journal write", call: insertAt(1), fault: fault{op: "WriteFile", base: renumberJournalName + ".tmp", times: 1}},
		{name: "insert: journal rename", call: insertAt(1), fault: fault{op: "Rename", base: renumberJournalName + ".tmp", times: 1}},
		{name: "insert: moved card write", call: insertAt(1), fault: fault{op: "WriteFile", base: "05-delta.md", times: 1}},
		{name: "insert: last moved card write", call: insertAt(1), fault: fault{op: "WriteFile", base: "03-beta.md", times: 1}},
		{name: "insert: moved card remove", call: insertAt(1), fault: fault{op: "Remove", base: "04-delta.md", times: 1}},
		{name: "insert: new card write", call: insertAt(1), fault: fault{op: "WriteFile", base: "02-new.md", times: 1}},
		{name: "insert: Card Index rewrite", call: insertAt(1), fault: fault{op: "WriteFile", base: renumberOverview, times: 1}},
		{name: "insert: journal removal", call: insertAt(1), fault: fault{op: "Remove", base: renumberJournalName, times: 1}},
		{name: "remove: journal directory creation", call: removeAt(1), fault: fault{op: "MkdirAll", base: "sub", times: 1}},
		{name: "remove: temporary journal write", call: removeAt(1), fault: fault{op: "WriteFile", base: renumberJournalName + ".tmp", times: 1}},
		{name: "remove: journal rename", call: removeAt(1), fault: fault{op: "Rename", base: renumberJournalName + ".tmp", times: 1}},
		{name: "remove: deletion", call: removeAt(1), fault: fault{op: "Remove", base: "02-beta.md", times: 1}},
		{name: "remove: moved card write", call: removeAt(1), fault: fault{op: "WriteFile", base: "02-gamma.md", times: 1}},
		{name: "remove: moved card remove", call: removeAt(1), fault: fault{op: "Remove", base: "03-gamma.md", times: 1}},
		{name: "remove: Card Index rewrite", call: removeAt(1), fault: fault{op: "WriteFile", base: renumberOverview, times: 1}},
		{name: "remove: journal removal", call: removeAt(1), fault: fault{op: "Remove", base: renumberJournalName, times: 1}},
		{name: "insert: failure inside the reversal", call: insertAt(1), fault: fault{op: "WriteFile", base: renumberOverview, times: 2}, want: planparser.ErrRenumberReversalFailed},
		{name: "remove: failure inside the reversal", call: removeAt(1), fault: fault{op: "WriteFile", base: renumberOverview, times: 2}, want: planparser.ErrRenumberReversalFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRenumberFixture(t, 1)
			before := snapshot(t, fx.root)
			injected := tc.fault

			_, err := tc.call(faultyIO(&injected), fx)

			if tc.want == planparser.ErrRenumberReversalFailed {
				if !errors.Is(err, planparser.ErrRenumberReversalFailed) {
					t.Fatalf("error = %v; want ErrRenumberReversalFailed", err)
				}
				if _, statErr := os.Stat(fx.journalPath); statErr != nil {
					t.Errorf("journal not kept after a failed reversal: %v", statErr)
				}
				return
			}
			if !errors.Is(err, planparser.ErrRenumberIOFailed) {
				t.Fatalf("error = %v; want ErrRenumberIOFailed", err)
			}
			requireSameTree(t, before, snapshot(t, fx.root))
		})
	}
}
