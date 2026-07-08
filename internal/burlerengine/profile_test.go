// profile_test.go table-drives Profile.validate over the happy path and
// every fail-loud rule documented on validate: field-emptiness, path
// existence, FixScope legality, ClusterN's typed-error gate, and in-place
// absolute-path resolution for both relative and already-absolute entries.

package burlerengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newValidProfileFixture creates a temp worktree root with every file
// validate requires to exist (a target file, a target directory, a fasit
// file, and a pair of prior-round files) and returns the root plus a
// Profile that passes validate unmodified — each test mutates a copy of
// this base to exercise one rule at a time.
func newValidProfileFixture(t *testing.T) (root string, base Profile) {
	t.Helper()
	root = t.TempDir()

	writeFixtureFile(t, root, "target.txt", "target content")
	writeFixtureFile(t, root, "fasit.txt", "fasit content")
	writeFixtureFile(t, root, "prior-review.md", "prior review content")
	writeFixtureFile(t, root, "prior-fixer.md", "prior fixer content")
	if err := os.Mkdir(filepath.Join(root, "targetdir"), 0o755); err != nil {
		t.Fatalf("Mkdir(targetdir) = %v; want nil", err)
	}

	base = Profile{
		Target:            FileSet{Paths: []string{"target.txt"}},
		Fasit:             FileSet{Paths: []string{"fasit.txt"}},
		Rubric:            "the widget must be blue",
		FixScope:          FixScopeSource,
		ToolUse:           false,
		ClusterN:          0,
		ReviewPath:        "review.md",
		FixerReportPath:   "fixer-report.md",
		PriorReviews:      []string{"prior-review.md"},
		PriorFixerReports: []string{"prior-fixer.md"},
	}
	return root, base
}

// writeFixtureFile writes content to name under root, failing the test on
// any I/O error.
func writeFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", name, err)
	}
}

func TestProfile_Validate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(root string, p *Profile)
		wantErr   bool
		errSubstr string
		wantErrIs error
	}{
		{
			name:    "valid profile",
			mutate:  func(root string, p *Profile) {},
			wantErr: false,
		},
		{
			name: "target directory entry is valid",
			mutate: func(root string, p *Profile) {
				p.Target.Paths = []string{"targetdir"}
			},
			wantErr: false,
		},
		{
			name: "target instructions only",
			mutate: func(root string, p *Profile) {
				p.Target = FileSet{Instructions: "review the diff against main"}
			},
			wantErr: false,
		},
		{
			name: "fasit instructions only",
			mutate: func(root string, p *Profile) {
				p.Fasit = FileSet{Instructions: "judge against the discussion"}
			},
			wantErr: false,
		},
		{
			name: "target empty",
			mutate: func(root string, p *Profile) {
				p.Target = FileSet{}
			},
			wantErr:   true,
			errSubstr: "profile.Target must set at least one of Paths or Instructions",
		},
		{
			name: "target instructions whitespace only",
			mutate: func(root string, p *Profile) {
				p.Target = FileSet{Instructions: "   "}
			},
			wantErr:   true,
			errSubstr: "profile.Target must set at least one of Paths or Instructions",
		},
		{
			name: "fasit empty",
			mutate: func(root string, p *Profile) {
				p.Fasit = FileSet{}
			},
			wantErr:   true,
			errSubstr: "internal-consistency checking",
		},
		{
			name: "target path missing",
			mutate: func(root string, p *Profile) {
				p.Target.Paths = []string{"does-not-exist.txt"}
			},
			wantErr:   true,
			errSubstr: "profile.Target.Paths entry",
		},
		{
			name: "fasit path missing",
			mutate: func(root string, p *Profile) {
				p.Fasit.Paths = []string{"does-not-exist.txt"}
			},
			wantErr:   true,
			errSubstr: "profile.Fasit.Paths entry",
		},
		{
			name: "prior review path missing",
			mutate: func(root string, p *Profile) {
				p.PriorReviews = []string{"does-not-exist.md"}
			},
			wantErr:   true,
			errSubstr: "profile.PriorReviews entry",
		},
		{
			name: "prior fixer report path missing",
			mutate: func(root string, p *Profile) {
				p.PriorFixerReports = []string{"does-not-exist.md"}
			},
			wantErr:   true,
			errSubstr: "profile.PriorFixerReports entry",
		},
		{
			name: "rubric empty",
			mutate: func(root string, p *Profile) {
				p.Rubric = ""
			},
			wantErr:   true,
			errSubstr: "profile.Rubric must not be empty",
		},
		{
			name: "rubric whitespace only",
			mutate: func(root string, p *Profile) {
				p.Rubric = "   \n\t "
			},
			wantErr:   true,
			errSubstr: "profile.Rubric must not be empty",
		},
		{
			name: "fixscope empty",
			mutate: func(root string, p *Profile) {
				p.FixScope = ""
			},
			wantErr:   true,
			errSubstr: "profile.FixScope must be",
		},
		{
			name: "fixscope invalid",
			mutate: func(root string, p *Profile) {
				p.FixScope = "markdown"
			},
			wantErr:   true,
			errSubstr: "profile.FixScope must be",
		},
		{
			name: "clustern negative",
			mutate: func(root string, p *Profile) {
				p.ClusterN = -1
			},
			wantErr:   true,
			errSubstr: "profile.ClusterN must not be negative",
		},
		{
			name: "clustern positive is unsupported",
			mutate: func(root string, p *Profile) {
				p.ClusterN = 1
			},
			wantErr:   true,
			errSubstr: "cluster-N > 0 is not supported",
			wantErrIs: ErrClusterUnsupported,
		},
		{
			name: "reviewpath empty",
			mutate: func(root string, p *Profile) {
				p.ReviewPath = ""
			},
			wantErr:   true,
			errSubstr: "profile.ReviewPath must not be empty",
		},
		{
			name: "fixerreportpath empty",
			mutate: func(root string, p *Profile) {
				p.FixerReportPath = ""
			},
			wantErr:   true,
			errSubstr: "profile.FixerReportPath must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newValidProfileFixture(t)
			tt.mutate(root, &p)

			err := p.validate(root)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() = nil; want error containing %q", tt.errSubstr)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validate() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "burler: ") {
					t.Errorf("validate() error = %q; want burler: -prefixed message", err.Error())
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("errors.Is(validate() error, %v) = false; want true", tt.wantErrIs)
				}
				return
			}

			if err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
		})
	}
}

// TestProfile_Validate_ResolvesPathsInPlace asserts the happy path
// documented on validate: every path field is rewritten in place to a
// cleaned absolute path, already-absolute entries are kept verbatim, and
// relative entries are joined onto worktreeRoot.
func TestProfile_Validate_ResolvesPathsInPlace(t *testing.T) {
	root, p := newValidProfileFixture(t)

	// Mix a relative Fasit entry with an already-absolute one to prove
	// both branches of resolvePath run inside a single field.
	absoluteFasit := filepath.Join(root, "fasit.txt")
	p.Fasit.Paths = []string{"fasit.txt", absoluteFasit}

	if err := p.validate(root); err != nil {
		t.Fatalf("validate() = %v; want nil", err)
	}

	wantTarget := []string{filepath.Join(root, "target.txt")}
	if diffStrings(p.Target.Paths, wantTarget) {
		t.Errorf("Target.Paths = %v; want %v", p.Target.Paths, wantTarget)
	}

	wantFasit := []string{filepath.Join(root, "fasit.txt"), filepath.Clean(absoluteFasit)}
	if diffStrings(p.Fasit.Paths, wantFasit) {
		t.Errorf("Fasit.Paths = %v; want %v", p.Fasit.Paths, wantFasit)
	}

	wantPriorReviews := []string{filepath.Join(root, "prior-review.md")}
	if diffStrings(p.PriorReviews, wantPriorReviews) {
		t.Errorf("PriorReviews = %v; want %v", p.PriorReviews, wantPriorReviews)
	}

	wantPriorFixers := []string{filepath.Join(root, "prior-fixer.md")}
	if diffStrings(p.PriorFixerReports, wantPriorFixers) {
		t.Errorf("PriorFixerReports = %v; want %v", p.PriorFixerReports, wantPriorFixers)
	}

	wantReviewPath := filepath.Join(root, "review.md")
	if p.ReviewPath != wantReviewPath {
		t.Errorf("ReviewPath = %q; want %q", p.ReviewPath, wantReviewPath)
	}

	wantFixerReportPath := filepath.Join(root, "fixer-report.md")
	if p.FixerReportPath != wantFixerReportPath {
		t.Errorf("FixerReportPath = %q; want %q", p.FixerReportPath, wantFixerReportPath)
	}
}

// TestProfile_Validate_AbsolutePathsKeptVerbatim asserts that an
// already-absolute Target.Paths entry outside worktreeRoot survives
// validate unchanged (only filepath.Clean-ed), proving relative-vs-absolute
// handling is per-entry, not per-field.
func TestProfile_Validate_AbsolutePathsKeptVerbatim(t *testing.T) {
	root, p := newValidProfileFixture(t)

	elsewhere := t.TempDir()
	writeFixtureFile(t, elsewhere, "outside.txt", "outside content")
	absoluteTarget := filepath.Join(elsewhere, "outside.txt")
	p.Target.Paths = []string{absoluteTarget}

	if err := p.validate(root); err != nil {
		t.Fatalf("validate() = %v; want nil", err)
	}

	want := filepath.Clean(absoluteTarget)
	if len(p.Target.Paths) != 1 || p.Target.Paths[0] != want {
		t.Errorf("Target.Paths = %v; want [%q]", p.Target.Paths, want)
	}
}

// diffStrings reports whether got and want differ in length or content
// (order-sensitive — resolvePaths preserves input order).
func diffStrings(got, want []string) bool {
	if len(got) != len(want) {
		return true
	}
	for i := range got {
		if got[i] != want[i] {
			return true
		}
	}
	return false
}
