//go:build tier0

// targetholds_test.go covers TargetHolds, the rule that decides whether a card target holds a code location.

package planparser

import "testing"

func TestTargetHolds(t *testing.T) {
	t.Parallel()

	goPlan := &Plan{Language: "go"}
	nonePlan := &Plan{Language: "none"}
	tests := []struct {
		name   string
		plan   *Plan
		target string
		member string
		file   string
		want   bool
	}{
		{name: "member glyph itself", plan: goPlan, target: "internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "handle whose body is the member", plan: goPlan, target: "plan:internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "containing type", plan: goPlan, target: "internal/a#T", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "member target against its owning type", plan: goPlan, target: "internal/a#T.M", member: "internal/a#T", file: "internal/a/a.go"},
		{name: "method sibling", plan: goPlan, target: "internal/a#T.N", member: "internal/a#T.M", file: "internal/a/a.go"},
		{name: "unit self glyph", plan: goPlan, target: "internal/a#", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "external test member against its directory's unit self glyph", plan: goPlan, target: "internal/a#", member: "internal/a_test#TestX", file: "internal/a/a_test.go", want: true},
		{name: "file as a path", plan: goPlan, target: "internal/a/a.go", file: "internal/a/a.go", want: true},
		{name: "file as a self glyph", plan: goPlan, target: "internal/a/a.go#", file: "internal/a/a.go", want: true},
		{name: "file target against a found member in that file", plan: goPlan, target: "internal/a/a.go", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "handle whose body is the file's self glyph", plan: goPlan, target: "plan:internal/a/a.go#", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "another file", plan: goPlan, target: "internal/a/b.go#", file: "internal/a/a.go"},
		{name: "nil plan reads as the default language", target: "internal/a#T", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "language none holds a file path", plan: nonePlan, target: "internal/a/a.go", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "language none holds no member", plan: nonePlan, target: "internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TargetHolds(tt.plan, tt.target, tt.member, tt.file); got != tt.want {
				t.Errorf("TargetHolds(%q, %q, %q) = %v; want %v", tt.target, tt.member, tt.file, got, tt.want)
			}
		})
	}
}
