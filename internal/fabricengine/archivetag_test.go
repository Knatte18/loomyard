// archivetag_test.go — unit test for ArchiveTagSlug, the reader of the archive tag namespace.

package fabricengine

import "testing"

// TestArchiveTagSlug covers the accepted archive/<slug>/<sha> shape and the names that carry the prefix without a slug or tip, or another prefix.
func TestArchiveTagSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		tag      string
		wantSlug string
		wantOK   bool
	}{
		{"slug and tip", "archive/my-slug/0123456789ab", "my-slug", true},
		{"no tip segment", "archive/x", "", false},
		{"empty slug", "archive//abc", "", false},
		{"empty tip", "archive/x/", "", false},
		{"another prefix", "snapshot/x/abc", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			slug, ok := ArchiveTagSlug(tt.tag)
			if slug != tt.wantSlug || ok != tt.wantOK {
				t.Errorf("ArchiveTagSlug(%q) = (%q, %v); want (%q, %v)", tt.tag, slug, ok, tt.wantSlug, tt.wantOK)
			}
		})
	}
}
