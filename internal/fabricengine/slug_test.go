// slug_test.go pins the shared worktree-slug validator both Add and Remove route through.
// It is untagged Tier 1: validateWorktreeSlug is pure string work with no git spawn and no
// filesystem access.

package fabricengine

import (
	"strings"
	"testing"
)

// TestValidateWorktreeSlug covers every rejection class plus the accepting case, so a slug that can
// create a booby-trapped pair can never be handed to a teardown verb either.
func TestValidateWorktreeSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		slug          string
		junctionNames []string
		wantErrSubstr string
	}{
		{"ordinary slug", "my-task", nil, ""},
		{"empty", "", nil, "must not be empty"},
		{"whitespace only", "   ", nil, "must not be empty"},
		{"forward slash", "sub/dir", nil, "single path component"},
		{"backslash", `sub\dir`, nil, "single path component"},
		{"weft suffix", "zed-weft", nil, "reserved for weft worktrees"},
		{"board", "_board", nil, "reserved for lyx hub geometry"},
		{"portals", "_portals", nil, "reserved for lyx hub geometry"},
		{"launchers", "_launchers", nil, "reserved for lyx hub geometry"},
		{"durable structural dir", "_lyx", nil, "reserved for lyx hub geometry"},
		{"transient structural dir", ".lyx", nil, "reserved for lyx hub geometry"},
		{"configured junction name", "_extra", []string{"_extra"}, "reserved for lyx hub geometry"},
		// "." and ".." carry no separator and name no reserved directory, so every other rule here
		// waves them through — and filepath.Join then resolves ".." out of the launcher directory and
		// onto the hub itself, where Remove's os.RemoveAll deletes the whole hub.
		{"self reference", ".", nil, "not a relative path element"},
		{"parent reference", "..", nil, "not a relative path element"},
		{"trailing dot segment", "task/.", nil, "single path component"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateWorktreeSlug(tt.slug, tt.junctionNames)
			if tt.wantErrSubstr == "" {
				if err != nil {
					t.Errorf("validateWorktreeSlug(%q) = %v; want nil", tt.slug, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateWorktreeSlug(%q) = nil; want error containing %q", tt.slug, tt.wantErrSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("validateWorktreeSlug(%q) = %v; want error containing %q", tt.slug, err, tt.wantErrSubstr)
			}
			if !strings.Contains(err.Error(), "invalid slug") {
				t.Errorf("validateWorktreeSlug(%q) = %v; want error containing %q", tt.slug, err, "invalid slug")
			}
		})
	}
}
