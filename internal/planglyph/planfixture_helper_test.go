// planfixture_helper_test.go holds the on-disk plan fixture and card readers, the drift repair assertion and the delete-order site reader that the test tiers of this package share.

package planglyph

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// writePlanFixture writes a minimal, valid on-disk plan directory from cards.
// Cards are keyed by card number and valued by the card body markdown following the "# Card N — cardN" heading.
// It then parses the directory, returning both the directory and the parsed *planparser.Plan.
func writePlanFixture(t *testing.T, cards map[int]string) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()

	numbers := make([]int, 0, len(cards))
	for n := range cards {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	// plankit renders the overview and the Card Index; each card file is then replaced by its raw body, which is the part under test.
	spec := plankit.Plan{Approved: true, Language: "go", Framing: "framing"}
	for _, n := range numbers {
		spec.Cards = append(spec.Cards, plankit.Card{Number: n, Slug: fmt.Sprintf("card%d", n), Summary: "summary"})
	}
	files := plankit.Render(spec)
	for _, n := range numbers {
		slug := fmt.Sprintf("card%d", n)
		files[fmt.Sprintf("%02d-%s.md", n, slug)] = []byte(fmt.Sprintf("# Card %d — %s\n\n%s\n", n, slug, cards[n]))
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", path, err)
		}
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

const driftTestTimestamp = "2026-01-01T00:00:00Z"

// requireExactTierRepair asserts an exact-tier rename sub#Old -> sub#New was auto-repaired in card 1 of the plan under dir: the card names the new glyph only, and the amendments file carries exactly one entry holding every string of wantAmendment.
func requireExactTierRepair(t *testing.T, dir string, wantAmendment []string) {
	t.Helper()

	got := readCardFile(t, dir, 1, "card1")
	if strings.Contains(got, "sub#Old") {
		t.Errorf("card still references the old glyph after auto-repair: %s", got)
	}
	if !strings.Contains(got, "sub#New") {
		t.Errorf("card was not rewritten to the new glyph: %s", got)
	}

	amendData, readErr := os.ReadFile(filepath.Join(dir, planparser.AmendmentsFileName))
	if readErr != nil {
		t.Fatalf("read amendments file: %v", readErr)
	}
	amendText := string(amendData)
	for _, want := range wantAmendment {
		if !strings.Contains(amendText, want) {
			t.Errorf("amendments file = %q; want it to contain %q", amendText, want)
		}
	}
	if strings.Count(amendText, "OldGlyph:") != 1 {
		t.Errorf("amendments file = %q; want exactly one amendment entry", amendText)
	}
}

// blockingDeleteOrderSite and informationalDeleteOrderSite pick the location out of a delete-before-reference detail.
var (
	blockingDeleteOrderSite      = regexp.MustCompile(`still references it at (\S+:\d+) in its Edit code`)
	informationalDeleteOrderSite = regexp.MustCompile(`follows a dot in (.+), in the Edit code of card`)
)

// deleteOrderSites returns, sorted, "<severity> <location>" for each delete-before-reference finding of findings, the location read from its detail: "<file>:<line>" when blocking and "<file> at line <n>" when informational.
// A finding whose detail holds no location yields its whole detail, so a changed wording fails the comparison instead of passing unseen.
func deleteOrderSites(findings []Finding) []string {
	var sites []string
	for _, f := range findings {
		if f.Check != "delete-before-reference" {
			continue
		}
		pattern := blockingDeleteOrderSite
		if f.Severity == SeverityInformational {
			pattern = informationalDeleteOrderSite
		}
		location := f.Detail
		if match := pattern.FindStringSubmatch(f.Detail); match != nil {
			location = match[1]
		}
		sites = append(sites, fmt.Sprintf("%s %s", f.Severity, location))
	}
	slices.Sort(sites)
	return sites
}
