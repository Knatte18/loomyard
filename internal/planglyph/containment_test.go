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
