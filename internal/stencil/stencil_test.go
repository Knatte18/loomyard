// stencil_test.go is the black-box contract test for stencil.Fill and stencil.FillOptional:
// one table covers the happy path, the unfilled-top-level-marker guard (including sorting/dedup),
// the incremental branch-internal guard, conditional sections, the leading-comment strip, the
// no-HTML-escaping guarantee, and FillOptional's optional-marker exemption from both guards.

package stencil_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// fillCase is one row of TestFill: a template and values rendered through stencil.Fill, or through
// stencil.FillOptional when optional is non-nil.
type fillCase struct {
	name       string
	template   string
	values     map[string]string
	optional   []string
	wantOutput string
	wantErr    bool
	// errExact, errContains and errLacks each apply only when non-empty, and only with wantErr.
	errExact    string
	errContains string
	errLacks    string
}

func render(tt fillCase) ([]byte, error) {
	if tt.optional != nil {
		return stencil.FillOptional([]byte(tt.template), tt.values, tt.optional)
	}
	return stencil.Fill([]byte(tt.template), tt.values)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestFill renders every row and asserts the exact output or the error shape.
// Every row must leave the caller's values map untouched, and rendered a second time must give
// byte-identical output and error text.
// A row without optional names is also rendered through FillOptional(template, values, nil), which
// must match Fill byte for byte.
func TestFill(t *testing.T) {
	t.Parallel()

	const branchTemplate = `{{if eq .Type "Cluster"}}Body: {{.Body}}{{end}}`

	tests := []fillCase{
		{
			name:       "single_marker",
			template:   "Fasit: {{.Fasit}}",
			values:     map[string]string{"Fasit": "the-answer"},
			wantOutput: "Fasit: the-answer",
		},
		{
			name:       "several_markers",
			template:   "Fasit: {{.Fasit}}\nTarget: {{.Target}}\n",
			values:     map[string]string{"Fasit": "foo", "Target": "bar"},
			wantOutput: "Fasit: foo\nTarget: bar\n",
		},
		{
			name:       "extra_keys_ignored",
			template:   "Fasit: {{.Fasit}}",
			values:     map[string]string{"Fasit": "x", "Extra": "y", "Another": "z"},
			wantOutput: "Fasit: x",
		},
		{
			name:       "no_html_escaping",
			template:   "Value: {{.Val}}",
			values:     map[string]string{"Val": `<b>&"'</b>`},
			wantOutput: `Value: <b>&"'</b>`,
		},
		{
			name:       "empty_template",
			template:   "",
			values:     map[string]string{},
			wantOutput: "",
		},
		{
			name:       "whitespace_only_template",
			template:   "   \n\t  ",
			values:     map[string]string{},
			wantOutput: "   \n\t  ",
		},
		{
			name:       "leading_comment_dropped_marker_inside_not_checked",
			template:   "<!-- Ghost: {{.Ghost}} -->\nFasit: {{.Fasit}}",
			values:     map[string]string{"Fasit": "present"},
			wantOutput: "Fasit: present",
		},
		{
			name:       "mid_template_comment_preserved_verbatim",
			template:   "Fasit: {{.Fasit}}\n<!-- note: this is fine -->\nDone.",
			values:     map[string]string{"Fasit": "x"},
			wantOutput: "Fasit: x\n<!-- note: this is fine -->\nDone.",
		},
		{
			name:       "comment_only_template_renders_empty",
			template:   "<!-- just a comment, nothing else -->",
			values:     map[string]string{},
			wantOutput: "",
		},
		{
			name:       "conditional_taken",
			template:   `Head{{if eq .Type "Cluster"}} Body: {{.Body}}{{end}} Tail`,
			values:     map[string]string{"Type": "Cluster", "Body": "inner"},
			wantOutput: "Head Body: inner Tail",
		},
		{
			name:       "conditional_not_taken_needs_no_branch_only_marker",
			template:   `Head{{if eq .Type "Cluster"}} Body: {{.Body}}{{end}} Tail`,
			values:     map[string]string{"Type": "Solo"},
			wantOutput: "Head Tail",
		},
		{
			name:       "branch_present_but_empty_not_taken_no_error",
			template:   `Head{{if .Active}} Body: {{.Body}}{{end}} Tail`,
			values:     map[string]string{"Active": "", "Body": ""},
			wantOutput: "Head Tail",
		},
		{
			name:        "absent_marker",
			template:    "Fasit: {{.Fasit}}\n",
			values:      map[string]string{},
			wantErr:     true,
			errContains: "Fasit",
		},
		{
			name:     "absent_marker_nil_values",
			template: "{{.missing}}",
			values:   nil,
			wantErr:  true,
			errExact: "stencil: unfilled top-level marker(s): missing",
		},
		{
			name:        "empty_string_value",
			template:    "Fasit: {{.Fasit}}",
			values:      map[string]string{"Fasit": ""},
			wantErr:     true,
			errContains: "Fasit",
		},
		{
			name:        "whitespace_only_value",
			template:    "Fasit: {{.Fasit}}",
			values:      map[string]string{"Fasit": "   "},
			wantErr:     true,
			errContains: "Fasit",
		},
		{
			name:     "multiple_offenders_sorted",
			template: "{{.Target}} {{.Fasit}} {{.Other}}",
			values:   map[string]string{"Other": "present"},
			wantErr:  true,
			errExact: "stencil: unfilled top-level marker(s): Fasit, Target",
		},
		{
			name:     "repeated_offender_deduped",
			template: "{{.Fasit}} and again {{.Fasit}}",
			values:   map[string]string{},
			wantErr:  true,
			errExact: "stencil: unfilled top-level marker(s): Fasit",
		},
		{
			name:        "branch_internal_absent",
			template:    branchTemplate,
			values:      map[string]string{"Type": "Cluster"},
			wantErr:     true,
			errContains: "Body",
		},
		{
			// Fill returns before execution reaches the branch, so the in-branch name never appears.
			name:        "top_level_offender_wins_over_branch_offender",
			template:    "Fasit: {{.Fasit}}\n" + branchTemplate,
			values:      map[string]string{"Type": "Cluster"},
			wantErr:     true,
			errContains: "Fasit",
			errLacks:    "Body",
		},
		{
			name:        "branch_present_but_empty_bare_field_condition",
			template:    `{{if .Active}}Body: {{.Body}}{{end}}`,
			values:      map[string]string{"Active": "yes", "Body": ""},
			wantErr:     true,
			errContains: "Body",
		},
		{
			name:        "branch_present_but_whitespace_only_eq_condition",
			template:    branchTemplate,
			values:      map[string]string{"Type": "Cluster", "Body": "   "},
			wantErr:     true,
			errContains: "Body",
		},
		{
			// The discriminator is absent from values, so the static guard adds nothing and the
			// execution-time missingkey=error path fires.
			name:     "branch_present_but_empty_unresolvable_condition_left_to_execution",
			template: branchTemplate,
			values:   map[string]string{"Body": ""},
			wantErr:  true,
		},
		{
			name:     "forgotten_discriminator",
			template: `{{if eq .Type "Cluster"}}Body{{end}}`,
			values:   map[string]string{},
			wantErr:  true,
		},
		{
			name:        "malformed_unclosed_if",
			template:    `{{if .X}}unclosed`,
			values:      map[string]string{"X": "y"},
			wantErr:     true,
			errContains: "parse template:",
		},
		{
			name:        "malformed_unclosed_action",
			template:    `{{.Fasit`,
			values:      map[string]string{},
			wantErr:     true,
			errContains: "parse template:",
		},
		{
			name:       "optional_absent_renders_nothing",
			template:   "Head: {{.Head}}\nExtra: {{.Extra}}",
			values:     map[string]string{"Head": "present"},
			optional:   []string{"Extra"},
			wantOutput: "Head: present\nExtra: ",
		},
		{
			name:       "optional_present_but_empty_renders_nothing",
			template:   "Extra: {{.Extra}}",
			values:     map[string]string{"Extra": ""},
			optional:   []string{"Extra"},
			wantOutput: "Extra: ",
		},
		{
			// The same TrimSpace definition of empty that governs the top-level guard governs the
			// optional seeding.
			name:       "optional_whitespace_only_normalises_to_empty",
			template:   "Extra: [{{.Extra}}]",
			values:     map[string]string{"Extra": "   "},
			optional:   []string{"Extra"},
			wantOutput: "Extra: []",
		},
		{
			name:       "optional_present_and_non_empty_renders_value",
			template:   "Extra: {{.Extra}}",
			values:     map[string]string{"Extra": "filled-in"},
			optional:   []string{"Extra"},
			wantOutput: "Extra: filled-in",
		},
		{
			name:       "optional_name_absent_from_template_is_no_op",
			template:   "Fasit: {{.Fasit}}",
			values:     map[string]string{"Fasit": "value"},
			optional:   []string{"NeverMentioned"},
			wantOutput: "Fasit: value",
		},
		{
			name:       "optional_branch_internal_present_but_empty_no_error",
			template:   `{{if .Active}}Body: {{.Body}}{{end}}`,
			values:     map[string]string{"Active": "yes", "Body": ""},
			optional:   []string{"Body"},
			wantOutput: "Body: ",
		},
		{
			name:        "non_optional_empty_marker_still_errors",
			template:    "Fasit: {{.Fasit}}",
			values:      map[string]string{"Fasit": ""},
			optional:    []string{"SomeOtherName"},
			wantErr:     true,
			errContains: "Fasit",
		},
		{
			// The exemption removes a name from the offenders list rather than suppressing the
			// whole error.
			name:     "mix_of_optional_and_required_empty_reports_only_required",
			template: "Fasit: {{.Fasit}}\nExtra: {{.Extra}}",
			values:   map[string]string{"Fasit": "", "Extra": ""},
			optional: []string{"Extra"},
			wantErr:  true,
			errExact: "stencil: unfilled top-level marker(s): Fasit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			valuesBefore := maps.Clone(tt.values)
			got, err := render(tt)
			if !maps.Equal(tt.values, valuesBefore) {
				t.Errorf("values map mutated by the call: now %v, was %v", tt.values, valuesBefore)
			}

			again, againErr := render(tt)
			if string(got) != string(again) || errText(err) != errText(againErr) {
				t.Errorf("second call = (%q, %q); first call = (%q, %q); want identical",
					string(again), errText(againErr), string(got), errText(err))
			}
			if tt.optional == nil {
				viaOptional, optionalErr := stencil.FillOptional([]byte(tt.template), tt.values, nil)
				if string(got) != string(viaOptional) || errText(err) != errText(optionalErr) {
					t.Errorf("FillOptional(nil) = (%q, %q); Fill = (%q, %q); want byte-identical",
						string(viaOptional), errText(optionalErr), string(got), errText(err))
				}
			}

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(got) != tt.wantOutput {
					t.Errorf("output = %q; want %q", string(got), tt.wantOutput)
				}
				return
			}

			if err == nil {
				t.Fatalf("got nil error, output %q; want an error", got)
			}
			if tt.errExact != "" && err.Error() != tt.errExact {
				t.Errorf("error = %q; want %q", err.Error(), tt.errExact)
			}
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error = %q; want substring %q", err.Error(), tt.errContains)
			}
			if tt.errLacks != "" && strings.Contains(err.Error(), tt.errLacks) {
				t.Errorf("error = %q; must not contain %q", err.Error(), tt.errLacks)
			}
		})
	}
}
