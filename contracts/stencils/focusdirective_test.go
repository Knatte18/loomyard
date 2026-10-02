// focusdirective_test.go pins burler-focus-directive.md's marker set and the four rules of its rubric-over-directive precedence statement, as substring pins in rubric_test.go's shape.

package stencils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

func TestBurlerFocusDirective_MarkersAndPrecedenceRules(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(BurlerFocusDirective)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers() = %v; want nil", err)
	}
	if want := []string{"focus_path"}; !reflect.DeepEqual(markers, want) {
		t.Errorf("TopLevelMarkers = %v; want %v", markers, want)
	}

	body := string(BurlerFocusDirective)
	for _, phrase := range []string{
		"The rubric binds over the focus directive",
		"steers attention and order, not verdicts",
		"a valid verdict",
		"Severity comes from the rubric's mapping",
		"advisory and yields to evidence",
		"`exclude_lenses` keeps its mechanical meaning",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("burler-focus-directive.md does not contain %q", phrase)
		}
	}
}

func TestFocusDepartures_ReviewFormatAndJudgeRatification(t *testing.T) {
	if !strings.Contains(string(BurlerStep2Review), "## Focus departures") {
		t.Errorf("burler-step-2-review.md does not contain %q", "## Focus departures")
	}
	if !strings.Contains(string(BouncerTemplateJudge), "ratify or reject each departure explicitly") {
		t.Errorf("bouncer-template-judge.md does not contain the ratify-or-reject instruction")
	}
}

func TestBouncerStencils_FocusEntryGuidance(t *testing.T) {
	stencils := map[string][]byte{
		"bouncer-template-seed.md":  BouncerTemplateSeed,
		"bouncer-template-judge.md": BouncerTemplateJudge,
	}
	for name, body := range stencils {
		for _, phrase := range []string{
			"never caps severity",
			"never pre-states a verdict",
			"against the rubric's `Do not flag` list",
		} {
			if !strings.Contains(string(body), phrase) {
				t.Errorf("%s does not contain %q", name, phrase)
			}
		}
	}
}

func TestBouncerJudge_FocusEntryIsSelfContained(t *testing.T) {
	body := string(BouncerTemplateJudge)
	for _, phrase := range []string{
		"restates every site, commit and claim it depends on",
		"never refers the reviewer to a prior round's review, fixer report, or finding ID",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("bouncer-template-judge.md does not contain %q", phrase)
		}
	}
}

func TestBurlerFocusDirective_NeverLicensesPriorRoundReads(t *testing.T) {
	body := string(BurlerFocusDirective)
	for _, phrase := range []string{
		"never licenses reading a prior round's files before your review is saved",
		"followed only as far as its own text goes",
		"`## Focus departures`",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("burler-focus-directive.md does not contain %q", phrase)
		}
	}
}
