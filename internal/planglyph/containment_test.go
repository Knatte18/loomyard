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
