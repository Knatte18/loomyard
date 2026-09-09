// containment_test.go covers resolveContainment against a small fixture repository.

package planglyph

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

func TestResolveContainment_MemberAndOwnFileSelfOnTwoCardsOverlap(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo", "sub/a.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	plan := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		{Number: 2, Slug: "two", Targets: []string{"sub/a.go#"}},
	}}

	got := resolveContainment(plan, results)
	if len(got) != 1 || got[0].Check != "containment-file-overlap" || got[0].Card != "1-one" {
		t.Fatalf("resolveContainment(...) = %+v; want exactly one containment-file-overlap finding on card 1-one", got)
	}
}

// TestResolveContainment_MultipleOverlapsAreDeterministicallyOrdered proves the finding order is
// stable across runs. resolveContainment walks a map, and a Go map range is randomised, so a plan
// carrying several overlaps used to render its findings in a different order on every call.
func TestResolveContainment_MultipleOverlapsAreDeterministicallyOrdered(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Foo() {}\n\nfunc Bar() {}\n\nfunc Baz() {}\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo", "sub#Bar", "sub#Baz", "sub/a.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	plan := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		{Number: 2, Slug: "two", Targets: []string{"sub#Bar"}},
		{Number: 3, Slug: "three", Targets: []string{"sub#Baz"}},
		{Number: 4, Slug: "four", Targets: []string{"sub/a.go#"}},
	}}

	first := resolveContainment(plan, results)
	if len(first) != 3 {
		t.Fatalf("resolveContainment(...) = %+v; want three containment-file-overlap findings", first)
	}
	// Repeat enough times that a randomised map walk would almost certainly diverge at least once.
	for i := 0; i < 32; i++ {
		again := resolveContainment(plan, results)
		if len(again) != len(first) {
			t.Fatalf("run %d returned %d findings; first run returned %d", i, len(again), len(first))
		}
		for j := range first {
			if again[j] != first[j] {
				t.Fatalf("run %d finding %d = %+v; first run had %+v — ordering is not deterministic", i, j, again[j], first[j])
			}
		}
	}
}

func TestResolveContainment_SameCardNoFinding(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo", "sub/a.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	plan := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#Foo", "sub/a.go#"}},
	}}

	got := resolveContainment(plan, results)
	if len(got) != 0 {
		t.Errorf("resolveContainment(same card) = %+v; want no findings", got)
	}
}

func TestResolveContainment_UnrelatedFileSelfNoFinding(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Foo() {}\n",
		"sub/b.go": "package sub\n\nfunc Bar() {}\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo", "sub/b.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	plan := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		{Number: 2, Slug: "two", Targets: []string{"sub/b.go#"}},
	}}

	got := resolveContainment(plan, results)
	if len(got) != 0 {
		t.Errorf("resolveContainment(unrelated file) = %+v; want no findings", got)
	}
}

func TestResolveContainment_MultipartSecondPartOverlaps(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc init() {}\n",
		"sub/b.go": "package sub\n\nfunc init() {}\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#init", "sub/b.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if results[0].Status != quarry.StatusMultipart {
		t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, quarry.StatusMultipart)
	}

	plan := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#init"}},
		{Number: 2, Slug: "two", Targets: []string{"sub/b.go#"}},
	}}

	got := resolveContainment(plan, results)
	if len(got) != 1 || got[0].Check != "containment-file-overlap" {
		t.Fatalf("resolveContainment(multipart) = %+v; want exactly one containment-file-overlap finding", got)
	}
}

// TestResolveContainment_ReadOnlyRefsAreNotAContainmentHazard is the regression test for R6-3:
// containment is a WRITE hazard, so a card that only READS a file and a card that only READS a symbol
// living in it must produce no finding. Building the overlap index from Targets+Uses made every such
// pair a SeverityBlocking finding, which refuses `lyx webster run` outright and every dispatch after
// it — on a plan carrying nothing but ordinary read-only references.
func TestResolveContainment_ReadOnlyRefsAreNotAContainmentHazard(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo", "sub/a.go#"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	readsOnly := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Uses: []string{"sub#Foo"}},
		{Number: 2, Slug: "two", Uses: []string{"sub/a.go#"}},
	}}
	if got := resolveContainment(readsOnly, results); len(got) != 0 {
		t.Errorf("resolveContainment(reads only) = %+v; want no findings — reading overlapping things conflicts with nothing", got)
	}

	// One writer and one reader is equally safe: nothing serializes on a read.
	writeThenRead := &planparser.Plan{Cards: []planparser.Card{
		{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		{Number: 2, Slug: "two", Uses: []string{"sub/a.go#"}},
	}}
	if got := resolveContainment(writeThenRead, results); len(got) != 0 {
		t.Errorf("resolveContainment(one writer, one reader) = %+v; want no findings", got)
	}
}

// TestResolveContainment_UnreadableStatusFailsClosed is card 10's regression for the Status half of
// resolveContainment's member-target disposition: a synthetic ResolveResult carrying a Status
// outside quarry's four-value vocabulary must surface the blocking glyph-rejected finding rather
// than silently drop the member from the containment index. Both an out-of-vocabulary Status string
// and the zero-value Status (quarry's own pre-resolution rejection shape, carrying Error and Reason
// instead of a Status) are exercised, since both routes fall through to the same default arm.
func TestResolveContainment_UnreadableStatusFailsClosed(t *testing.T) {
	plan := &planparser.Plan{
		Language: "go",
		Cards: []planparser.Card{
			{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		},
	}

	t.Run("OutOfVocabularyStatus", func(t *testing.T) {
		results := []quarry.ResolveResult{{Target: "sub#Foo", Status: quarry.Status("weird")}}
		got := resolveContainment(plan, results)
		if len(got) != 1 || got[0].Check != "glyph-rejected" || got[0].Card != "1-one" || got[0].Severity != SeverityBlocking {
			t.Fatalf("resolveContainment(out-of-vocabulary Status) = %+v; want exactly one blocking glyph-rejected finding on card 1-one", got)
		}
	})

	t.Run("ZeroValueStatusWithErrorAndReason", func(t *testing.T) {
		results := []quarry.ResolveResult{{Target: "sub#Foo", Error: "boom", Reason: "unparseable target"}}
		got := resolveContainment(plan, results)
		if len(got) != 1 || got[0].Check != "glyph-rejected" || got[0].Card != "1-one" || got[0].Severity != SeverityBlocking {
			t.Fatalf("resolveContainment(zero-value Status) = %+v; want exactly one blocking glyph-rejected finding on card 1-one", got)
		}
	})
}

// TestResolveContainment_AbsentTargetSkipsSilently is card 10's pin for the OTHER half of the same
// disposition: a member target entirely absent from the results index (the !resolved branch) stays
// a silent skip, never a finding and never a containment entry, because canonicalization can rewrite
// a plan: handle into a glyph ref that this pass's own resolve batch never carried. This is the
// legal case the Status-half fail-closed guard above deliberately does not touch.
func TestResolveContainment_AbsentTargetSkipsSilently(t *testing.T) {
	plan := &planparser.Plan{
		Language: "go",
		Cards: []planparser.Card{
			{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
		},
	}

	got := resolveContainment(plan, nil)
	if len(got) != 0 {
		t.Errorf("resolveContainment(absent target) = %+v; want no findings — absence is legal here", got)
	}
}
