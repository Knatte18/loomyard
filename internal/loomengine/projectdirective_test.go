//go:build tier0

// projectdirective_test.go covers ProjectRecordDirective over the seeded stencils.

package loomengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func TestProjectRecordDirective(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	t.Run("NoRecordsRendersNothing", func(t *testing.T) {
		t.Parallel()
		got, err := ProjectRecordDirective(dir, nil)
		if err != nil || got != "" {
			t.Errorf("ProjectRecordDirective(nil) = %q, %v; want empty, nil", got, err)
		}
	})

	t.Run("RecordsRenderAsBulletsInOrder", func(t *testing.T) {
		t.Parallel()
		records := []string{"/hub/inner/_lym/project/inner/decision-record.md", "/hub/outer/_lym/project/outer/decision-record.md"}
		got, err := ProjectRecordDirective(dir, records)
		if err != nil {
			t.Fatalf("ProjectRecordDirective() error = %v; want nil", err)
		}
		want := "- `" + records[0] + "`\n- `" + records[1] + "`"
		if !strings.Contains(got, want) {
			t.Errorf("directive lacks the record bullets %q in order:\n%s", want, got)
		}
		if strings.Contains(got, "{{") || strings.Contains(got, "<!--") {
			t.Errorf("directive keeps a marker or its banner:\n%s", got)
		}
	})

	t.Run("MissingStencilIsAnError", func(t *testing.T) {
		t.Parallel()
		if _, err := ProjectRecordDirective(t.TempDir(), []string{"/record.md"}); err == nil || !strings.Contains(err.Error(), projectDirectiveStencil) {
			t.Errorf("ProjectRecordDirective() over an empty directory error = %v; want one naming %s", err, projectDirectiveStencil)
		}
	})
}
