// follow_test.go table-drives FollowTemplate: what a file gains, changes and loses against its template base, and the operator values it keeps.

package yamlengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

func TestFollowTemplate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const longFlowMapping = "m: {alpha: aaaaaaaaaaaaaaaaaaaa, beta: bbbbbbbbbbbbbbbbbbbb, gamma: cccccccccccccccccccc, delta: dddddddddddddddd}\n"
	tests := []struct {
		name     string
		base     string
		haveBase bool
		template string
		existing string
		want     string
		report   yamlengine.FollowReport
	}{
		{
			name:     "a unit at the base's value takes the template's new value and keeps its comments",
			base:     "a: 1\n",
			haveBase: true,
			template: "a: 2\n",
			existing: "# note\na: 1 # mine\n",
			want:     "# note\na: 2 # mine\n",
			report:   yamlengine.FollowReport{Updated: []string{"a: 1 -> 2"}},
		},
		{
			name:     "a unit the template dropped leaves while the file holds the base's value",
			base:     "p:\n  x: 1\nq: 1\n",
			haveBase: true,
			template: "q: 1\n",
			existing: "p:\n  x: 1\nq: 1\n",
			want:     "q: 1\n",
			report:   yamlengine.FollowReport{Removed: []string{"p"}},
		},
		{
			name:     "a removal that empties a mapping removes the mapping",
			base:     "p:\n  x: 1\nq: 1\n",
			haveBase: true,
			template: "p: {}\nq: 1\n",
			existing: "p:\n  x: 1\nq: 1\n",
			want:     "q: 1\n",
			report:   yamlengine.FollowReport{Removed: []string{"p.x"}},
		},
		{
			name:     "a subtree the template added is copied in its style without the template's comments",
			haveBase: true,
			template: "# head\nm: {a: 1, b: [x]}\n",
			existing: "k: v\n",
			want:     "k: v\nm: {a: 1, b: [x]}\n",
			report:   yamlengine.FollowReport{Added: []string{"m"}},
		},
		{
			name:     "a key the template added joins a flow mapping the file holds",
			base:     "p: {a: 1}\n",
			haveBase: true,
			template: "p: {a: 1, b: \"\"}\n",
			existing: "p: {a: 5}\n",
			want:     "p: {a: 5, b: \"\"}\n",
			report:   yamlengine.FollowReport{Added: []string{"p.b"}},
		},
		{
			name:     "a value the operator set is kept and listed when the template moved it to a third value",
			base:     "a: 1\n",
			haveBase: true,
			template: "a: 2\n",
			existing: "a: 3\n",
			want:     "a: 3\n",
			report:   yamlengine.FollowReport{Differs: []string{"a: file 3, template 2"}},
		},
		{
			name:     "a value the operator set is kept silently while the template holds the base's value",
			base:     "a: 1\n",
			haveBase: true,
			template: "a: 1\n",
			existing: "a: 3\n",
			want:     "a: 3\n",
		},
		{
			name:     "a unit the operator removed stays removed and is listed when the template moved it",
			base:     "a: 1\nb: 1\n",
			haveBase: true,
			template: "a: 2\nb: 1\n",
			existing: "c: 1\n",
			want:     "c: 1\n",
			report:   yamlengine.FollowReport{Differs: []string{"a: file absent, template 2"}},
		},
		{
			name:     "without a base a lacking unit is added, a differing one kept and listed, and an extra one kept silently",
			template: "a: 1\nb: 2\n",
			existing: "a: 5\nc: 3\n",
			want:     "a: 5\nc: 3\nb: 2\n",
			report:   yamlengine.FollowReport{Added: []string{"b"}, Differs: []string{"a: file 5, template 1"}},
		},
		{
			name:     "a scalar the template turned into a mapping is one unit",
			base:     "a: 1\n",
			haveBase: true,
			template: "a: {x: 1}\n",
			existing: "a: 1\n",
			want:     "a: {x: 1}\n",
			report:   yamlengine.FollowReport{Updated: []string{"a: 1 -> {x: 1}"}},
		},
		{
			name:     "a sequence at the base's value takes the template's new sequence as one unit",
			base:     "a: [1, 2]\n",
			haveBase: true,
			template: "a: [1, 3]\n",
			existing: "a: [1, 2]\n",
			want:     "a: [1, 3]\n",
			report:   yamlengine.FollowReport{Updated: []string{"a: [1, 2] -> [1, 3]"}},
		},
		{
			name:     "a sequence the operator edited is kept whole and listed",
			base:     "a: [1, 2]\n",
			haveBase: true,
			template: "a: [1, 3]\n",
			existing: "a: [1, 2, 9]\n",
			want:     "a: [1, 2, 9]\n",
			report:   yamlengine.FollowReport{Differs: []string{"a: file [1, 2, 9], template [1, 3]"}},
		},
		{
			name:     "a changed file keeps a flow mapping past 80 columns on one line",
			base:     "a: 1\n",
			haveBase: true,
			template: "a: 2\n",
			existing: "a: 1\n\n" + longFlowMapping,
			want:     "a: 2\n" + longFlowMapping,
			report:   yamlengine.FollowReport{Updated: []string{"a: 1 -> 2"}},
		},
		{
			name:     "a base that is not a mapping is not read while the file has no base",
			base:     "- not\n- a mapping\n",
			template: "a: 1\n",
			existing: "a: 1\n",
			want:     "a: 1\n",
		},
		{
			name:     "an empty file gains the template",
			template: "a: 1\n",
			want:     "a: 1\n",
			report:   yamlengine.FollowReport{Added: []string{"a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, report, err := yamlengine.FollowTemplate([]byte(tt.base), []byte(tt.template), []byte(tt.existing), tt.haveBase)
			if err != nil {
				t.Fatalf("FollowTemplate() error = %v; want none", err)
			}
			if string(got) != tt.want {
				t.Errorf("FollowTemplate() = %q; want %q", got, tt.want)
			}
			for _, field := range []struct {
				name      string
				got, want []string
			}{
				{"Added", report.Added, tt.report.Added},
				{"Updated", report.Updated, tt.report.Updated},
				{"Removed", report.Removed, tt.report.Removed},
				{"Differs", report.Differs, tt.report.Differs},
			} {
				if !slices.Equal(field.got, field.want) {
					t.Errorf("FollowTemplate() %s = %q; want %q", field.name, field.got, field.want)
				}
			}
			if wantChanged := tt.want != tt.existing; report.Changed() != wantChanged {
				t.Errorf("FollowTemplate() Changed = %v; want %v", report.Changed(), wantChanged)
			}
		})
	}

	if _, _, err := yamlengine.FollowTemplate(nil, []byte("a: 1\n"), []byte("- 1\n"), false); err == nil {
		t.Error("FollowTemplate(a list document) error = nil; want a refusal naming the top level")
	}
}
