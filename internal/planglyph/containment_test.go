// containment_test.go covers resolveContainment against a small fixture repository.

package planglyph

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestResolveContainment_Overlaps covers which target pairs the containment index flags: a member
// and its own file on two cards overlap, the second part of a multipart member overlaps its file,
// and the same card, an unrelated file and read-only references never do.
// Containment is a WRITE hazard: a card that only READS a file and a card that only READS a symbol
// living in it must produce no finding, and neither does one writer beside one reader.
// Building the overlap index from Targets+Uses made every such pair a SeverityBlocking finding,
// which refuses the whole run on a plan carrying nothing but ordinary read-only references.
func TestResolveContainment_Overlaps(t *testing.T) {
	t.Parallel()

	oneFoo := map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"}

	cases := []struct {
		name    string
		files   map[string]string
		targets []string
		// firstStatus, when set, is the status the fixture must give targets[0].
		firstStatus quarry.Status
		cards       []planparser.Card
		// wantCard names the one card carrying the containment-file-overlap finding; empty means no finding.
		wantCard string
	}{
		{
			name:    "member and own file on two cards",
			files:   oneFoo,
			targets: []string{"sub#Foo", "sub/a.go#"},
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
				{Number: 2, Slug: "two", Targets: []string{"sub/a.go#"}},
			},
			wantCard: "1-one",
		},
		{
			name: "second part of a multipart member overlaps",
			files: map[string]string{
				"sub/a.go": "package sub\n\nfunc init() {}\n",
				"sub/b.go": "package sub\n\nfunc init() {}\n",
			},
			targets:     []string{"sub#init", "sub/b.go#"},
			firstStatus: quarry.StatusMultipart,
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Targets: []string{"sub#init"}},
				{Number: 2, Slug: "two", Targets: []string{"sub/b.go#"}},
			},
			wantCard: "1-one",
		},
		{
			name:    "same card",
			files:   oneFoo,
			targets: []string{"sub#Foo", "sub/a.go#"},
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Targets: []string{"sub#Foo", "sub/a.go#"}},
			},
		},
		{
			name: "unrelated file",
			files: map[string]string{
				"sub/a.go": "package sub\n\nfunc Foo() {}\n",
				"sub/b.go": "package sub\n\nfunc Bar() {}\n",
			},
			targets: []string{"sub#Foo", "sub/b.go#"},
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
				{Number: 2, Slug: "two", Targets: []string{"sub/b.go#"}},
			},
		},
		{
			name:    "reads only",
			files:   oneFoo,
			targets: []string{"sub#Foo", "sub/a.go#"},
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Uses: []string{"sub#Foo"}},
				{Number: 2, Slug: "two", Uses: []string{"sub/a.go#"}},
			},
		},
		{
			name:    "one writer and one reader",
			files:   oneFoo,
			targets: []string{"sub#Foo", "sub/a.go#"},
			cards: []planparser.Card{
				{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}},
				{Number: 2, Slug: "two", Uses: []string{"sub/a.go#"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := writeFixtureRepo(t, tc.files)
			repo, err := openRepo(root)
			if err != nil {
				t.Fatalf("openRepo(%q) returned error: %v", root, err)
			}
			results, err := resolveTargets(repo, tc.targets)
			if err != nil {
				t.Fatalf("resolveTargets(...) returned error: %v", err)
			}
			if tc.firstStatus != "" && results[0].Status != tc.firstStatus {
				t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, tc.firstStatus)
			}

			got := resolveContainment(&planparser.Plan{Cards: tc.cards}, results)
			if tc.wantCard == "" {
				if len(got) != 0 {
					t.Errorf("resolveContainment(...) = %+v; want no findings", got)
				}
				return
			}
			if len(got) != 1 || got[0].Check != "containment-file-overlap" || got[0].Card != tc.wantCard {
				t.Fatalf("resolveContainment(...) = %+v; want exactly one containment-file-overlap finding on card %s", got, tc.wantCard)
			}
		})
	}
}

// TestResolveContainment_MultipleOverlapsAreDeterministicallyOrdered proves the finding order is
// stable across runs. resolveContainment walks a map, and a Go map range is randomised, so a plan
// carrying several overlaps used to render its findings in a different order on every call.
//
//testtiming:keep pins that the finding order is identical across repeated runs, which TestResolveContainment_Overlaps does not assert
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
