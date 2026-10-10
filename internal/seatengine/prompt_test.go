// prompt_test.go covers the seat value maps, the include-filled prompts and the shuttle specs a table composes.

package seatengine

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// promptGeometry returns a geometry whose stencils directory holds the shipped seat blocks and the test seat stencils.
func promptGeometry(t *testing.T) Geometry {
	t.Helper()
	return Geometry{
		WorktreeRoot: "/w",
		AnchorPath:   "/anchor",
		StencilsDir:  seatedStencils(t),
		ParentName:   "ly:orch",
		Shortname:    "ly",
		Slug:         "task",
	}
}

// promptTable returns a chair with two advisors, carrying the settings every spec reads.
func promptTable() Table {
	table := validTable()
	table.Timeout = 7 * time.Minute
	table.Interactive = true
	table.Values = map[string]string{"topic": "the topic"}
	table.Seats[0].Inputs = []string{"/w/a1.md", "/w/a2.md"}
	table.Seats[0].Model, table.Seats[0].Effort, table.Seats[0].Version = "opus", "high", "5"
	table.Seats[0].Skills = []string{"scribe:prose"}
	table.Seats[1].Inputs = []string{"/w/in.md"}
	table.Seats[1].Values = map[string]string{"focus": "the focus"}
	return table
}

func TestSeatPrompts_ValuesPromptsAndSpecs(t *testing.T) {
	t.Parallel()
	geom := promptGeometry(t)
	table := promptTable()

	names, err := strandNames(geom, table)
	if err != nil {
		t.Fatalf("strandNames() = %v", err)
	}
	wantNames := map[string]string{
		RoleChair:      "ly:task:multi-chair",
		AdvisorName(1): "ly:task:multi-advisor-1",
		AdvisorName(2): "ly:task:multi-advisor-2",
	}
	for seat, want := range wantNames {
		if names[seat] != want {
			t.Errorf("strandNames()[%q] = %q, want %q", seat, names[seat], want)
		}
	}

	chairValues, err := seatValues(geom, table, table.Seats[0], names, []string{names[AdvisorName(2)]})
	if err != nil {
		t.Fatalf("seatValues(chair) = %v", err)
	}
	advisorValues, err := seatValues(geom, table, table.Seats[1], names, []string{names[AdvisorName(2)]})
	if err != nil {
		t.Fatalf("seatValues(advisor) = %v", err)
	}
	for label, values := range map[string]map[string]string{"chair": chairValues, "advisor": advisorValues} {
		for _, marker := range ReservedMarkers() {
			if values[marker] == "" {
				t.Errorf("%s values: reserved marker %q is empty", label, marker)
			}
		}
		if values["topic"] != "the topic" || values["worktree_root"] != "/w" || values["anchor_path"] != "/anchor" || values["stencils_dir"] != geom.StencilsDir {
			t.Errorf("%s values = %v, want the table value and the three roots", label, values)
		}
		if want := "ly:task:multi-advisor-1\nly:task:multi-advisor-2"; values["advisor_names"] != want {
			t.Errorf("%s advisor_names = %q, want %q", label, values["advisor_names"], want)
		}
		if values["chair_name"] != "ly:task:multi-chair" {
			t.Errorf("%s chair_name = %q, want the chair's strand name", label, values["chair_name"])
		}
	}

	if got := chairValues["failed_advisors"]; got != "ly:task:multi-advisor-2" {
		t.Errorf("chair failed_advisors = %q, want the failed advisor", got)
	}
	if got := advisorValues["failed_advisors"]; got != noneValue {
		t.Errorf("advisor failed_advisors = %q, want %q", got, noneValue)
	}
	if got := chairValues["inputs"]; got != "/w/a1.md\n/w/a2.md" {
		t.Errorf("chair inputs = %q, want the chair's inputs one per line", got)
	}
	if got := chairValues["outputs"]; got != "/w/out.md" || chairValues["output_files"] != got {
		t.Errorf("chair outputs = %q, output_files = %q, want the chair's output in both", got, chairValues["output_files"])
	}
	if got := advisorValues["focus"]; got != "the focus" {
		t.Errorf("advisor focus = %q, want the seat's own value", got)
	}

	emptyInputs, err := seatValues(geom, table, table.Seats[2], names, nil)
	if err != nil {
		t.Fatalf("seatValues(advisor with no inputs) = %v", err)
	}
	if emptyInputs["inputs"] != noneValue || emptyInputs["failed_advisors"] != noneValue {
		t.Errorf("empty lists = %q and %q, want %q for both", emptyInputs["inputs"], emptyInputs["failed_advisors"], noneValue)
	}

	if !strings.Contains(chairValues["parent_directive"], "`ly:orch`") {
		t.Errorf("chair parent_directive = %q, want it to name the run's parent", chairValues["parent_directive"])
	}
	if strings.Contains(chairValues["parent_directive"], "never the way forward") {
		t.Errorf("chair parent_directive = %q, want no operator ban for an interactive chair", chairValues["parent_directive"])
	}
	if !strings.Contains(advisorValues["parent_directive"], "`ly:task:multi-chair`") || !strings.Contains(advisorValues["parent_directive"], "never the way forward") {
		t.Errorf("advisor parent_directive = %q, want the chair as parent and the operator ban", advisorValues["parent_directive"])
	}

	chairPrompt, err := composePrompt(geom.StencilsDir, table.Seats[0], chairValues)
	if err != nil {
		t.Fatalf("composePrompt(chair) = %v", err)
	}
	for _, want := range []string{"Chair of ly:task:multi-chair.", "You are the chair of this step", "ly:task:multi-advisor-1\nly:task:multi-advisor-2", "/w/out.md"} {
		if !strings.Contains(chairPrompt, want) {
			t.Errorf("chair prompt lacks %q:\n%s", want, chairPrompt)
		}
	}
	advisorPrompt, err := composePrompt(geom.StencilsDir, table.Seats[1], advisorValues)
	if err != nil {
		t.Fatalf("composePrompt(advisor) = %v", err)
	}
	for _, want := range []string{"Advisor ly:task:multi-advisor-1.", "You are an advisor in this step", "ly:task:multi-chair", "/w/a1.md"} {
		if !strings.Contains(advisorPrompt, want) {
			t.Errorf("advisor prompt lacks %q:\n%s", want, advisorPrompt)
		}
	}

	chairSpec := seatSpec(table, table.Seats[0], chairPrompt)
	if chairSpec.Prompt != chairPrompt || !slices.Equal(chairSpec.OutputFiles, []string{"/w/out.md"}) {
		t.Errorf("chair spec prompt/outputs = %q / %v, want the prompt and the chair's outputs", chairSpec.Prompt, chairSpec.OutputFiles)
	}
	if chairSpec.Model != "opus" || chairSpec.Effort != "high" || chairSpec.Version != "5" {
		t.Errorf("chair spec model triple = %q/%q/%q, want opus/high/5", chairSpec.Model, chairSpec.Effort, chairSpec.Version)
	}
	if !chairSpec.Interactive || chairSpec.KeepPane || chairSpec.QuietHold {
		t.Errorf("chair spec Interactive/KeepPane/QuietHold = %v/%v/%v, want true/false/false", chairSpec.Interactive, chairSpec.KeepPane, chairSpec.QuietHold)
	}
	if !slices.Equal(chairSpec.Skills, []string{"scribe:prose"}) {
		t.Errorf("chair spec skills = %v, want the seat's skills", chairSpec.Skills)
	}

	advisorSpec := seatSpec(table, table.Seats[1], advisorPrompt)
	if advisorSpec.Interactive || !advisorSpec.KeepPane || !advisorSpec.QuietHold {
		t.Errorf("advisor spec Interactive/KeepPane/QuietHold = %v/%v/%v, want false/true/true", advisorSpec.Interactive, advisorSpec.KeepPane, advisorSpec.QuietHold)
	}
	for label, spec := range map[string]struct {
		role, segment string
		timeout       time.Duration
	}{
		"chair":   {chairSpec.Role, string(chairSpec.Segment), chairSpec.Timeout},
		"advisor": {advisorSpec.Role, string(advisorSpec.Segment), advisorSpec.Timeout},
	} {
		wantRole := "multi-" + label
		if label == "advisor" {
			wantRole = "multi-advisor-1"
		}
		if spec.role != wantRole || spec.segment != "plan" || spec.timeout != 7*time.Minute {
			t.Errorf("%s spec role/segment/timeout = %q/%q/%v, want %q/plan/7m", label, spec.role, spec.segment, spec.timeout, wantRole)
		}
	}
}

func TestComposePrompt_RefusesAMarkerAbsentFromTheValueMap(t *testing.T) {
	t.Parallel()
	geom := promptGeometry(t)
	writeStencil(t, geom.StencilsDir, "seat-test-undeclared", "Needs {{.undeclared_marker}}.\n{{template \"seat-directive-advisor\"}}\n")
	table := promptTable()
	names, err := strandNames(geom, table)
	if err != nil {
		t.Fatalf("strandNames() = %v", err)
	}
	seat := table.Seats[1]
	seat.Stencil = "seat-test-undeclared"
	values, err := seatValues(geom, table, seat, names, nil)
	if err != nil {
		t.Fatalf("seatValues() = %v", err)
	}

	_, err = composePrompt(geom.StencilsDir, seat, values)
	if err == nil || !strings.Contains(err.Error(), "undeclared_marker") {
		t.Fatalf("composePrompt() = %v, want an error naming undeclared_marker", err)
	}
}
