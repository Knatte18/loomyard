package loomengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newPriorPlanStencilsDir returns a seeded stencils directory.
func newPriorPlanStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

func TestPriorPlanBlock_RendersHeadingArchiveAndBullets(t *testing.T) {
	dir := newPriorPlanStencilsDir(t)
	archive := filepath.Join(t.TempDir(), "archive", "plan-1")
	files := []string{"00-overview.md", "01-alpha.md"}

	got, err := PriorPlanBlock(dir, archive, files)
	if err != nil {
		t.Fatalf("PriorPlanBlock() error = %v; want nil", err)
	}
	if !strings.Contains(got, "Prior plan") {
		t.Errorf("block lacks the Prior plan heading:\n%s", got)
	}
	if !strings.Contains(got, archive) {
		t.Errorf("block lacks archive dir %q:\n%s", archive, got)
	}
	for _, f := range files {
		if !strings.Contains(got, "\n- `"+f+"`") {
			t.Errorf("block lacks bullet for %q:\n%s", f, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("block has unrendered marker:\n%s", got)
	}
}

func TestPriorPlanBlock_EmptyInputsError(t *testing.T) {
	dir := newPriorPlanStencilsDir(t)
	tests := []struct {
		name    string
		archive string
		files   []string
		want    string
	}{
		{"empty archive dir", "", []string{"a.md"}, "archive_dir"},
		{"empty moved files", "/x/archive", nil, "moved_files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PriorPlanBlock(dir, tt.archive, tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("PriorPlanBlock() error = %v; want it to name %q", err, tt.want)
			}
		})
	}
}

func TestPriorPlanBlock_MissingStencilErrors(t *testing.T) {
	if _, err := PriorPlanBlock(t.TempDir(), "/x/archive", []string{"a.md"}); err == nil {
		t.Error("PriorPlanBlock() with no stencil error = nil; want an error")
	}
}
