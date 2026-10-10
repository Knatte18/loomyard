// buildenv_test.go covers classifyAmbiguity and the constraint text it describes each candidate
// with, table-driven over Go files written to a temp root.

package planglyph

import (
	"slices"
	"testing"

	"github.com/Knatte18/quarry/quarry"
)

// TestClassifyAmbiguity asserts an ambiguous answer is partitioned only when no build environment
// selects two candidate files, and that every candidate is described whatever the verdict.
func TestClassifyAmbiguity(t *testing.T) {
	t.Parallel()

	const body = "\n\npackage p\n"
	tests := []struct {
		name            string
		files           map[string]string
		candidateFiles  []string
		wantPartitioned bool
		wantConstraints []string
	}{
		{
			name:            "go:build linux beside go:build !linux",
			files:           map[string]string{"a.go": "//go:build linux" + body, "b.go": "//go:build !linux" + body},
			candidateFiles:  []string{"a.go", "b.go"},
			wantPartitioned: true,
			wantConstraints: []string{"//go:build linux", "//go:build !linux"},
		},
		{
			name:            "linux filename suffix beside windows filename suffix",
			files:           map[string]string{"x_linux.go": "package p\n", "x_windows.go": "package p\n"},
			candidateFiles:  []string{"x_linux.go", "x_windows.go"},
			wantPartitioned: true,
			wantConstraints: []string{"_linux.go", "_windows.go"},
		},
		{
			name:            "go:build unix beside windows filename suffix",
			files:           map[string]string{"a.go": "//go:build unix" + body, "x_windows.go": "package p\n"},
			candidateFiles:  []string{"a.go", "x_windows.go"},
			wantPartitioned: true,
			wantConstraints: []string{"//go:build unix", "_windows.go"},
		},
		{
			name:            "go:build linux beside go:build amd64",
			files:           map[string]string{"a.go": "//go:build linux" + body, "b.go": "//go:build amd64" + body},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build linux", "//go:build amd64"},
		},
		{
			name:            "go:build !windows beside go:build unix",
			files:           map[string]string{"a.go": "//go:build !windows" + body, "b.go": "//go:build unix" + body},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build !windows", "//go:build unix"},
		},
		{
			name:            "release tag beside an unconstrained file",
			files:           map[string]string{"a.go": "//go:build !go1.21" + body, "b.go": "package p\n"},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build !go1.21", "no constraint"},
		},
		{
			name:            "compiler tag beside an unconstrained file",
			files:           map[string]string{"a.go": "//go:build !gc" + body, "b.go": "package p\n"},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build !gc", "no constraint"},
		},
		{
			name:            "plus-build linux beside plus-build !linux",
			files:           map[string]string{"a.go": "// +build linux" + body, "b.go": "// +build !linux" + body},
			candidateFiles:  []string{"a.go", "b.go"},
			wantPartitioned: true,
			wantConstraints: []string{"// +build linux", "// +build !linux"},
		},
		{
			name: "five free tags",
			files: map[string]string{
				"a.go": "//go:build t1 && t2 && t3 && t4 && t5" + body,
				"b.go": "//go:build !t1" + body,
			},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build t1 && t2 && t3 && t4 && t5", "//go:build !t1"},
		},
		{
			name:            "unreadable candidate file",
			files:           map[string]string{"b.go": "//go:build !linux" + body},
			candidateFiles:  []string{"gone.go", "b.go"},
			wantConstraints: []string{"no constraint", "//go:build !linux"},
		},
		{
			name:            "unparseable go:build line",
			files:           map[string]string{"a.go": "//go:build linux &&" + body, "b.go": "//go:build !linux" + body},
			candidateFiles:  []string{"a.go", "b.go"},
			wantConstraints: []string{"//go:build linux &&", "//go:build !linux"},
		},
		{
			name:            "one candidate",
			files:           map[string]string{"a.go": "//go:build linux" + body},
			candidateFiles:  []string{"a.go"},
			wantConstraints: []string{"//go:build linux"},
		},
		{
			name:            "two candidates in one file",
			files:           map[string]string{"a.go": "//go:build linux" + body},
			candidateFiles:  []string{"a.go", "a.go"},
			wantConstraints: []string{"//go:build linux", "//go:build linux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFixtureFiles(t, root, tt.files)
			candidates := make([]quarry.Symbol, len(tt.candidateFiles))
			for i, file := range tt.candidateFiles {
				candidates[i] = quarry.Symbol{ID: "p#Member", File: file}
			}

			got := classifyAmbiguity(root, candidates)

			if got.Partitioned != tt.wantPartitioned {
				t.Errorf("classifyAmbiguity(%v).Partitioned = %v; want %v", tt.candidateFiles, got.Partitioned, tt.wantPartitioned)
			}
			gotConstraints := make([]string, len(got.Candidates))
			for i, candidate := range got.Candidates {
				gotConstraints[i] = candidate.Constraint
			}
			if !slices.Equal(gotConstraints, tt.wantConstraints) {
				t.Errorf("classifyAmbiguity(%v) constraints = %q; want %q", tt.candidateFiles, gotConstraints, tt.wantConstraints)
			}
		})
	}
}
