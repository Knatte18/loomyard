// add_test.go — unit test proving Add is wired to slug validation.
// slug_test.go owns the rejection classes.
// Validation runs before any git operation, so this needs no git fixture and stays untagged Tier-1.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestAdd_WiresSlugValidation(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		wantRefuse bool
	}{
		{"SeparatorRefused", "nested/slug", true},
		{"JunctionNameRefused", "_extra", true},
		{"RaddleAccepted", "_raddle", false},
		{"DotLyxRefused", ".lyx", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _extra"})
			// The layout points at a non-repo temp dir: validation must run before Add consults it,
			// so no git error can mask the validation error.
			layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

			_, err := topology.Add(layout, tt.slug, fabricengine.AddOptions{})
			if got := errors.Is(err, fabricengine.ErrInvalidSlug); got != tt.wantRefuse {
				t.Errorf("Add(%q) error = %v; errors.Is(ErrInvalidSlug) = %v, want %v", tt.slug, err, got, tt.wantRefuse)
			}
		})
	}
}
