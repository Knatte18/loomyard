// refnames_test.go covers the readers and composers of the weft sibling name and the archive tag names.

package fabriclayout

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWeftWarpSlug checks that WeftWarpSlug round-trips a weft sibling directory name,
// and that it rejects names without the weftname.Suffix tail or with an empty stripped prefix.
func TestWeftWarpSlug(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name     string
		input    string
		wantSlug string
		wantOK   bool
	}{
		{"plain_slug", "feat-weft", "feat", true},
		{"nested_prefix_slug", "hanf/foo-weft", "hanf/foo", true},
		{"no_suffix", "feat", "", false},
		{"suffix_only_empty_prefix", "-weft", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlug, gotOK := WeftWarpSlug(tt.input)
			if gotOK != tt.wantOK {
				t.Errorf("WeftWarpSlug(%q) ok = %v; want %v", tt.input, gotOK, tt.wantOK)
			}
			if gotSlug != tt.wantSlug {
				t.Errorf("WeftWarpSlug(%q) = %q; want %q", tt.input, gotSlug, tt.wantSlug)
			}
		})
	}
}

// TestArchiveTagSlug covers the accepted archive/<slug>/<sha> shape.
// It also covers the names that carry the prefix without a slug or tip, and those with another prefix.
func TestArchiveTagSlug(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestOwnerArchiveTagName pins the owner archive tag's name,
// and that no archive tag reader takes it for an archived run.
func TestOwnerArchiveTagName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tag := OwnerArchiveTagName("root", "0123456789abcdef0123")
	if want := "archive-owner/root/0123456789ab"; tag != want {
		t.Errorf("OwnerArchiveTagName() = %q; want %q", tag, want)
	}
	if slug, ok := ArchiveTagSlug(tag); ok {
		t.Errorf("ArchiveTagSlug(%q) = (%q, true); want no archived run", tag, slug)
	}
	if strings.HasPrefix(tag, ArchiveTagPrefix) {
		t.Errorf("%q carries the archive tag prefix %q; want it outside every archive tag listing", tag, ArchiveTagPrefix)
	}
}
