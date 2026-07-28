// stencil_test.go is the black-box, table-driven contract test for stencil.Fill and
// stencil.FillOptional: the happy path, the unfilled-top-level-marker guard (including
// sorting/dedup), the incremental branch-internal guard, conditional sections, the
// leading-comment strip, the no-HTML-escaping / idempotence guarantees, and
// FillOptional's optional-marker exemption from both guards.

package stencil_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// fillCase is the shared table-row shape for the scenarios that reduce to "given this
// template and these values, either the exact output or an error substring results."
type fillCase struct {
	name          string
	template      string
	values        map[string]string
	wantOutput    string // checked only when checkOutput is true
	checkOutput   bool
	wantErr       bool
	wantErrSubstr string // substring the error must contain, checked only when wantErr
}

// runFillCases exercises stencil.Fill for every row in tests, asserting either the
// exact rendered output or that an error containing wantErrSubstr was returned.
func runFillCases(t *testing.T, tests []fillCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stencil.Fill([]byte(tt.template), tt.values)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Fill() got nil error; want error containing %q", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("Fill() error = %q; want substring %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Fill() unexpected error: %v", err)
			}
			if tt.checkOutput && string(got) != tt.wantOutput {
				t.Errorf("Fill() = %q; want %q", string(got), tt.wantOutput)
			}
		})
	}
}

// TestFill_HappyPath covers several top-level {{.X}} markers, all present and
// non-empty, rendering the correct substituted output with a nil error.
func TestFill_HappyPath(t *testing.T) {
	tests := []fillCase{
		{
			name:        "single_marker",
			template:    "Fasit: {{.Fasit}}",
			values:      map[string]string{"Fasit": "the-answer"},
			wantOutput:  "Fasit: the-answer",
			checkOutput: true,
		},
		{
			name:        "several_markers",
			template:    "Fasit: {{.Fasit}}\nTarget: {{.Target}}\n",
			values:      map[string]string{"Fasit": "foo", "Target": "bar"},
			wantOutput:  "Fasit: foo\nTarget: bar\n",
			checkOutput: true,
		},
	}
	runFillCases(t, tests)
}

// TestFill_MissingTopLevelMarker covers a referenced top-level marker absent from
// values, which must error naming that marker.
func TestFill_MissingTopLevelMarker(t *testing.T) {
	tests := []fillCase{
		{
			name:          "absent_marker",
			template:      "Fasit: {{.Fasit}}\n",
			values:        map[string]string{},
			wantErr:       true,
			wantErrSubstr: "Fasit",
		},
		{
			name:          "absent_marker_nil_values",
			template:      "Target: {{.Target}}\n",
			values:        nil,
			wantErr:       true,
			wantErrSubstr: "Target",
		},
	}
	runFillCases(t, tests)
}

// TestFill_EmptyValue covers a top-level marker present as "" and as
// whitespace-only ("   "): both must error, the empty-fasit guard.
func TestFill_EmptyValue(t *testing.T) {
	tests := []fillCase{
		{
			name:          "empty_string",
			template:      "Fasit: {{.Fasit}}",
			values:        map[string]string{"Fasit": ""},
			wantErr:       true,
			wantErrSubstr: "Fasit",
		},
		{
			name:          "whitespace_only",
			template:      "Fasit: {{.Fasit}}",
			values:        map[string]string{"Fasit": "   "},
			wantErr:       true,
			wantErrSubstr: "Fasit",
		},
	}
	runFillCases(t, tests)
}

// TestFill_MultipleOffendersSortedAndDeduped covers two or more unfilled top-level
// markers collapsing into a single error listing all of them in sorted order, with a
// repeated reference to the same marker deduplicated to one entry.
func TestFill_MultipleOffendersSortedAndDeduped(t *testing.T) {
	got, err := stencil.Fill(
		[]byte("{{.Target}} {{.Fasit}} {{.Other}}"),
		map[string]string{"Other": "present"},
	)
	if err == nil {
		t.Fatalf("Fill() got nil error, output %q; want an unfilled-marker error", got)
	}
	wantMsg := "stencil: unfilled top-level marker(s): Fasit, Target"
	if err.Error() != wantMsg {
		t.Errorf("Fill() error = %q; want %q", err.Error(), wantMsg)
	}

	// The same marker referenced twice at top level must be reported once, not twice.
	_, dupErr := stencil.Fill([]byte("{{.Fasit}} and again {{.Fasit}}"), map[string]string{})
	if dupErr == nil {
		t.Fatalf("Fill() got nil error; want an unfilled-marker error for a repeated marker")
	}
	wantDupMsg := "stencil: unfilled top-level marker(s): Fasit"
	if dupErr.Error() != wantDupMsg {
		t.Errorf("Fill() error = %q; want %q (deduped)", dupErr.Error(), wantDupMsg)
	}
}

// TestFill_BranchInternalMissCaughtIncrementally covers a taken branch referencing an
// absent marker (error naming it), and a mix of an absent top-level marker plus an
// absent in-branch marker, where the top-level check reports first and the
// in-branch name never appears (Fill returns before execution reaches the branch).
func TestFill_BranchInternalMissCaughtIncrementally(t *testing.T) {
	t.Run("branch_internal_absent", func(t *testing.T) {
		_, err := stencil.Fill(
			[]byte(`{{if eq .Type "Cluster"}}Body: {{.Body}}{{end}}`),
			map[string]string{"Type": "Cluster"},
		)
		if err == nil {
			t.Fatal("Fill() got nil error; want an error naming the absent in-branch marker Body")
		}
		if !strings.Contains(err.Error(), "Body") {
			t.Errorf("Fill() error = %q; want it to name Body", err.Error())
		}
	})

	t.Run("top_level_offender_wins_over_branch_offender", func(t *testing.T) {
		_, err := stencil.Fill(
			[]byte("Fasit: {{.Fasit}}\n"+`{{if eq .Type "Cluster"}}Body: {{.Body}}{{end}}`),
			map[string]string{"Type": "Cluster"}, // Fasit absent, Body absent
		)
		if err == nil {
			t.Fatal("Fill() got nil error; want the top-level Fasit error")
		}
		if !strings.Contains(err.Error(), "Fasit") {
			t.Errorf("Fill() error = %q; want it to name Fasit", err.Error())
		}
		if strings.Contains(err.Error(), "Body") {
			t.Errorf("Fill() error = %q; must not name the in-branch marker Body (execution never ran)", err.Error())
		}
	})
}

// TestFill_MalformedTemplate covers an unparseable template (an unclosed {{if}}),
// which must return a non-nil error wrapping the parse failure, never panic.
func TestFill_MalformedTemplate(t *testing.T) {
	tests := []fillCase{
		{
			name:          "unclosed_if",
			template:      `{{if .X}}unclosed`,
			values:        map[string]string{"X": "y"},
			wantErr:       true,
			wantErrSubstr: "parse template:",
		},
		{
			name:          "unclosed_action",
			template:      `{{.Fasit`,
			values:        map[string]string{},
			wantErr:       true,
			wantErrSubstr: "parse template:",
		},
	}
	runFillCases(t, tests)
}

// TestFill_ConditionalTaken covers {{if eq .Type "Cluster"}}...{{end}} with a matching
// Type, where the section is present and its inner markers are substituted.
func TestFill_ConditionalTaken(t *testing.T) {
	tests := []fillCase{
		{
			name:        "cluster_branch_taken",
			template:    `Head{{if eq .Type "Cluster"}} Body: {{.Body}}{{end}} Tail`,
			values:      map[string]string{"Type": "Cluster", "Body": "inner"},
			wantOutput:  "Head Body: inner Tail",
			checkOutput: true,
		},
	}
	runFillCases(t, tests)
}

// TestFill_ConditionalNotTaken covers the same template with a non-matching Type: the
// section is absent, and markers living only inside that branch are not required (no
// error even though the branch-only marker is absent from values).
func TestFill_ConditionalNotTaken(t *testing.T) {
	got, err := stencil.Fill(
		[]byte(`Head{{if eq .Type "Cluster"}} Body: {{.Body}}{{end}} Tail`),
		map[string]string{"Type": "Solo"}, // Body deliberately absent
	)
	if err != nil {
		t.Fatalf("Fill() unexpected error: %v", err)
	}
	if strings.Contains(string(got), "Body:") {
		t.Errorf("Fill() = %q; the untaken branch must not render", string(got))
	}
	want := "Head Tail"
	if string(got) != want {
		t.Errorf("Fill() = %q; want %q", string(got), want)
	}
}

// TestFill_ForgottenDiscriminator covers a template that references
// {{if eq .Type ...}} while values has no Type key at all: the condition is always
// evaluated, so this must error rather than silently treat Type as false/empty.
func TestFill_ForgottenDiscriminator(t *testing.T) {
	_, err := stencil.Fill(
		[]byte(`{{if eq .Type "Cluster"}}Body{{end}}`),
		map[string]string{},
	)
	if err == nil {
		t.Fatal("Fill() got nil error; want an error for the missing Type discriminator")
	}
}

// TestFill_UnusedValuesIgnored covers values carrying keys the template never
// references: no error, output unaffected.
func TestFill_UnusedValuesIgnored(t *testing.T) {
	tests := []fillCase{
		{
			name:        "extra_keys_ignored",
			template:    "Fasit: {{.Fasit}}",
			values:      map[string]string{"Fasit": "x", "Extra": "y", "Another": "z"},
			wantOutput:  "Fasit: x",
			checkOutput: true,
		},
	}
	runFillCases(t, tests)
}

// TestFill_LeadingCommentStrip covers a leading <!-- ... --> being dropped (a marker
// inside it is neither substituted nor checked, so no error even though it references
// an undefined value), a mid-template comment preserved verbatim, and a comment-only
// template rendering empty output.
func TestFill_LeadingCommentStrip(t *testing.T) {
	t.Run("leading_comment_dropped_marker_inside_not_checked", func(t *testing.T) {
		got, err := stencil.Fill(
			[]byte("<!-- Ghost: {{.Ghost}} -->\nFasit: {{.Fasit}}"),
			map[string]string{"Fasit": "present"}, // Ghost is deliberately absent
		)
		if err != nil {
			t.Fatalf("Fill() unexpected error: %v", err)
		}
		want := "Fasit: present"
		if string(got) != want {
			t.Errorf("Fill() = %q; want %q", string(got), want)
		}
	})

	t.Run("mid_template_comment_preserved_verbatim", func(t *testing.T) {
		got, err := stencil.Fill(
			[]byte("Fasit: {{.Fasit}}\n<!-- note: this is fine -->\nDone."),
			map[string]string{"Fasit": "x"},
		)
		if err != nil {
			t.Fatalf("Fill() unexpected error: %v", err)
		}
		want := "Fasit: x\n<!-- note: this is fine -->\nDone."
		if string(got) != want {
			t.Errorf("Fill() = %q; want %q", string(got), want)
		}
	})

	t.Run("comment_only_template_renders_empty", func(t *testing.T) {
		got, err := stencil.Fill([]byte("<!-- just a comment, nothing else -->"), map[string]string{})
		if err != nil {
			t.Fatalf("Fill() unexpected error: %v", err)
		}
		if string(got) != "" {
			t.Errorf("Fill() = %q; want empty output", string(got))
		}
	})
}

// TestFill_EmptyOrWhitespaceOnlyTemplate covers an empty template and a
// whitespace-only template, both of which must render with a nil error and no
// substantive content.
func TestFill_EmptyOrWhitespaceOnlyTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{"empty", ""},
		{"whitespace_only", "   \n\t  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stencil.Fill([]byte(tt.template), map[string]string{})
			if err != nil {
				t.Fatalf("Fill() unexpected error: %v", err)
			}
			if strings.TrimSpace(string(got)) != "" {
				t.Errorf("Fill() = %q; want empty (or whitespace-only) output", string(got))
			}
		})
	}
}

// TestFill_IdempotenceAndDeterminism covers repeated calls with the same template and
// values producing byte-identical output, and the multi-offender error message being
// stable (sorted) across repeated calls.
func TestFill_IdempotenceAndDeterminism(t *testing.T) {
	t.Run("output_stable_across_calls", func(t *testing.T) {
		template := []byte("Fasit: {{.Fasit}}\nTarget: {{.Target}}\n")
		values := map[string]string{"Fasit": "foo", "Target": "bar"}

		first, err := stencil.Fill(template, values)
		if err != nil {
			t.Fatalf("Fill() unexpected error on first call: %v", err)
		}
		second, err := stencil.Fill(template, values)
		if err != nil {
			t.Fatalf("Fill() unexpected error on second call: %v", err)
		}
		if string(first) != string(second) {
			t.Errorf("Fill() not idempotent: first = %q, second = %q", string(first), string(second))
		}
	})

	t.Run("error_message_stable_across_calls", func(t *testing.T) {
		template := []byte("{{.Target}} {{.Fasit}}")
		values := map[string]string{}

		_, firstErr := stencil.Fill(template, values)
		_, secondErr := stencil.Fill(template, values)
		if firstErr == nil || secondErr == nil {
			t.Fatalf("Fill() got nil error(s); want unfilled-marker errors on both calls")
		}
		if firstErr.Error() != secondErr.Error() {
			t.Errorf("Fill() error message not stable: first = %q, second = %q", firstErr.Error(), secondErr.Error())
		}
	})
}

// TestFill_NoHTMLEscaping covers a value containing <, >, &, and quotes passing
// through verbatim, confirming Fill uses text/template rather than html/template.
func TestFill_NoHTMLEscaping(t *testing.T) {
	got, err := stencil.Fill(
		[]byte("Value: {{.Val}}"),
		map[string]string{"Val": `<b>&"'</b>`},
	)
	if err != nil {
		t.Fatalf("Fill() unexpected error: %v", err)
	}
	want := `Value: <b>&"'</b>`
	if string(got) != want {
		t.Errorf("Fill() = %q; want %q (no HTML escaping)", string(got), want)
	}
}

// TestFillOptional_AbsentRendersNothing covers a marker listed as optional and absent
// from values rendering as nothing with no error, instead of tripping the
// unfilled-top-level-marker guard.
func TestFillOptional_AbsentRendersNothing(t *testing.T) {
	got, err := stencil.FillOptional(
		[]byte("Head: {{.Head}}\nExtra: {{.Extra}}"),
		map[string]string{"Head": "present"},
		[]string{"Extra"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	want := "Head: present\nExtra: "
	if string(got) != want {
		t.Errorf("FillOptional() = %q; want %q", string(got), want)
	}
}

// TestFillOptional_PresentButEmptyRendersNothing covers a marker listed as optional and
// present in values as "" rendering as nothing with no error.
func TestFillOptional_PresentButEmptyRendersNothing(t *testing.T) {
	got, err := stencil.FillOptional(
		[]byte("Extra: {{.Extra}}"),
		map[string]string{"Extra": ""},
		[]string{"Extra"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	want := "Extra: "
	if string(got) != want {
		t.Errorf("FillOptional() = %q; want %q", string(got), want)
	}
}

// TestFillOptional_WhitespaceOnlyNormalisesToEmpty covers a marker listed as optional
// and present in values as whitespace-only rendering as nothing, not as its whitespace
// verbatim — the same TrimSpace-based "empty" definition unfilledTopLevelMarkers uses
// must also govern what FillOptional seeds before execution.
func TestFillOptional_WhitespaceOnlyNormalisesToEmpty(t *testing.T) {
	got, err := stencil.FillOptional(
		[]byte("Extra: [{{.Extra}}]"),
		map[string]string{"Extra": "   "},
		[]string{"Extra"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	want := "Extra: []"
	if string(got) != want {
		t.Errorf("FillOptional() = %q; want %q (whitespace-only optional value must normalise to empty)", string(got), want)
	}
}

// TestFillOptional_PresentAndNonEmptyRendersValue covers a marker listed as optional but
// present and non-empty rendering its actual value, confirming the optional exemption
// only changes behaviour for absent/empty values, not for a value that is genuinely set.
func TestFillOptional_PresentAndNonEmptyRendersValue(t *testing.T) {
	got, err := stencil.FillOptional(
		[]byte("Extra: {{.Extra}}"),
		map[string]string{"Extra": "filled-in"},
		[]string{"Extra"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	want := "Extra: filled-in"
	if string(got) != want {
		t.Errorf("FillOptional() = %q; want %q", string(got), want)
	}
}

// TestFillOptional_NonOptionalEmptyMarkerStillErrors covers a template with only a
// non-optional empty marker: the existing unfilled-top-level-marker error must still
// fire, confirming FillOptional(t, v, nil-equivalent-for-that-name) behaves exactly like
// Fill for names not listed as optional.
func TestFillOptional_NonOptionalEmptyMarkerStillErrors(t *testing.T) {
	_, err := stencil.FillOptional(
		[]byte("Fasit: {{.Fasit}}"),
		map[string]string{"Fasit": ""},
		[]string{"SomeOtherName"},
	)
	if err == nil {
		t.Fatal("FillOptional() got nil error; want the unfilled top-level marker error for the non-optional Fasit")
	}
	if !strings.Contains(err.Error(), "Fasit") {
		t.Errorf("FillOptional() error = %q; want it to name Fasit", err.Error())
	}
}

// TestFillOptional_MixOfOptionalAndRequiredEmptyReportsOnlyRequired covers a template
// with one optional-and-empty marker plus one required-and-empty marker: the error must
// name only the required one, confirming the optional exemption removes a name from the
// offenders list rather than merely suppressing the whole error.
func TestFillOptional_MixOfOptionalAndRequiredEmptyReportsOnlyRequired(t *testing.T) {
	_, err := stencil.FillOptional(
		[]byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}"),
		map[string]string{"Fasit": "", "Extra": ""},
		[]string{"Extra"},
	)
	if err == nil {
		t.Fatal("FillOptional() got nil error; want the unfilled top-level marker error for the non-optional Fasit")
	}
	wantMsg := "stencil: unfilled top-level marker(s): Fasit"
	if err.Error() != wantMsg {
		t.Errorf("FillOptional() error = %q; want %q (Extra must not appear, it is optional)", err.Error(), wantMsg)
	}
}

// TestFillOptional_ByteIdenticalToFillOnSameInput covers Fill(t, v) and
// FillOptional(t, v, nil) producing byte-identical output on the happy path and
// byte-identical error text on the error path, confirming Fill is genuinely defined as
// FillOptional(t, v, nil) rather than a parallel implementation that could drift.
func TestFillOptional_ByteIdenticalToFillOnSameInput(t *testing.T) {
	t.Run("happy_path", func(t *testing.T) {
		template := []byte("Fasit: {{.Fasit}}\nTarget: {{.Target}}\n")
		values := map[string]string{"Fasit": "foo", "Target": "bar"}

		fillGot, fillErr := stencil.Fill(template, values)
		optGot, optErr := stencil.FillOptional(template, values, nil)
		if fillErr != nil || optErr != nil {
			t.Fatalf("unexpected errors: Fill() = %v, FillOptional() = %v", fillErr, optErr)
		}
		if string(fillGot) != string(optGot) {
			t.Errorf("Fill() = %q; FillOptional(t, v, nil) = %q; want byte-identical", string(fillGot), string(optGot))
		}
	})

	t.Run("error_path", func(t *testing.T) {
		template := []byte("Fasit: {{.Fasit}}\n")
		values := map[string]string{}

		_, fillErr := stencil.Fill(template, values)
		_, optErr := stencil.FillOptional(template, values, nil)
		if fillErr == nil || optErr == nil {
			t.Fatalf("Fill() and FillOptional() must both error; got Fill() = %v, FillOptional() = %v", fillErr, optErr)
		}
		if fillErr.Error() != optErr.Error() {
			t.Errorf("Fill() error = %q; FillOptional(t, v, nil) error = %q; want byte-identical", fillErr.Error(), optErr.Error())
		}
	})
}

// TestFillOptional_OptionalNameAbsentFromTemplateIsNoOp covers an optional name listed
// but never referenced anywhere in the template: rendering succeeds unaffected, since
// there is no marker for the exemption to apply to.
func TestFillOptional_OptionalNameAbsentFromTemplateIsNoOp(t *testing.T) {
	got, err := stencil.FillOptional(
		[]byte("Fasit: {{.Fasit}}"),
		map[string]string{"Fasit": "value"},
		[]string{"NeverMentioned"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	want := "Fasit: value"
	if string(got) != want {
		t.Errorf("FillOptional() = %q; want %q", string(got), want)
	}
}

// TestFillOptional_CallerValuesMapNotMutated covers the caller's values map being left
// untouched after a call whose optional-seeding step would otherwise need to add or
// overwrite an entry, confirming FillOptional operates on a private copy.
func TestFillOptional_CallerValuesMapNotMutated(t *testing.T) {
	values := map[string]string{"Fasit": "value"}

	_, err := stencil.FillOptional(
		[]byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}"),
		values,
		[]string{"Extra"},
	)
	if err != nil {
		t.Fatalf("FillOptional() unexpected error: %v", err)
	}
	if _, exists := values["Extra"]; exists {
		t.Errorf("FillOptional() mutated the caller's values map by adding %q", "Extra")
	}
	if len(values) != 1 {
		t.Errorf("FillOptional() mutated the caller's values map; got %d entries, want 1", len(values))
	}
}

// TestFillOptional_RepeatedCallsProduceIdenticalOutput covers repeated FillOptional
// calls with the same inputs producing byte-identical output and identical error text,
// mirroring Fill's own idempotence guarantee.
func TestFillOptional_RepeatedCallsProduceIdenticalOutput(t *testing.T) {
	t.Run("output_stable_across_calls", func(t *testing.T) {
		template := []byte("Head: {{.Head}}\nExtra: {{.Extra}}")
		values := map[string]string{"Head": "value"}
		optional := []string{"Extra"}

		first, err := stencil.FillOptional(template, values, optional)
		if err != nil {
			t.Fatalf("FillOptional() unexpected error on first call: %v", err)
		}
		second, err := stencil.FillOptional(template, values, optional)
		if err != nil {
			t.Fatalf("FillOptional() unexpected error on second call: %v", err)
		}
		if string(first) != string(second) {
			t.Errorf("FillOptional() not idempotent: first = %q, second = %q", string(first), string(second))
		}
	})

	t.Run("error_text_stable_across_calls", func(t *testing.T) {
		template := []byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}")
		values := map[string]string{}
		optional := []string{"Extra"}

		_, firstErr := stencil.FillOptional(template, values, optional)
		_, secondErr := stencil.FillOptional(template, values, optional)
		if firstErr == nil || secondErr == nil {
			t.Fatalf("FillOptional() got nil error(s); want unfilled-marker errors on both calls")
		}
		if firstErr.Error() != secondErr.Error() {
			t.Errorf("FillOptional() error message not stable: first = %q, second = %q", firstErr.Error(), secondErr.Error())
		}
	})
}
