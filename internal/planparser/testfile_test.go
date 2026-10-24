//go:build tier0

// testfile_test.go table-tests the per-language test-file matchers, their union and the plan's effective language.

package planparser

import "testing"

func TestIsTestFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		language string
		path     string
		want     bool
	}{
		{"go test file", "go", "internal/x/foo_test.go", true},
		{"go source file", "go", "internal/x/foo.go", false},
		{"csharp Tests suffix", "csharp", "src/FooTests.cs", true},
		{"csharp Test suffix", "csharp", "src/FooTest.cs", true},
		{"csharp file under a Tests directory", "csharp", "src/BarTests/Baz.cs", true},
		{"csharp source file", "csharp", "src/Bar/Baz.cs", false},
		{"none reads every path as a test file", "none", "src/Bar/Baz.cs", true},
		{"unknown language reads every path as a test file", "rust", "src/lib.rs", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTestFile(tt.language, tt.path); got != tt.want {
				t.Errorf("IsTestFile(%q, %q) = %v; want %v", tt.language, tt.path, got, tt.want)
			}
		})
	}

	union := []struct {
		path string
		want bool
	}{
		{"internal/x/foo_test.go", true},
		{"src/FooTests.cs", true},
		{"internal/x/foo.go", false},
	}
	for _, tt := range union {
		if got := IsAnyLanguageTestFile(tt.path); got != tt.want {
			t.Errorf("IsAnyLanguageTestFile(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}

	languages := []struct {
		plan Plan
		want string
	}{
		{Plan{}, "go"},
		{Plan{Language: "go"}, "go"},
		{Plan{Language: "csharp"}, "csharp"},
	}
	for _, tt := range languages {
		if got := PlanLanguage(&tt.plan); got != tt.want {
			t.Errorf("PlanLanguage(language %q) = %q; want %q", tt.plan.Language, got, tt.want)
		}
	}
}
