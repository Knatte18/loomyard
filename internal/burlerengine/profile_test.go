// profile_test.go table-drives Profile.validate over the happy path and every fail-loud rule
// documented on validate: field-emptiness, path existence, FixScope legality, ClusterFan's
// fan-resolution gate, and in-place absolute-path resolution for both relative and already-absolute
// entries.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testClusterFanConfig returns a Config fixture exercising every ResolveFan
// outcome profile_test.go's table needs: a resolvable "standard" fan, a fan
// naming an undefined lens ("badlens"), and a fan longer than maxClusterN
// ("huge") — deliberately distinct from any fixture config_test.go itself
// owns, so this file's cases never depend on that file's fixtures.
func testClusterFanConfig() Config {
	huge := make([]string, maxClusterN+1)
	for i := range huge {
		huge[i] = "style"
	}
	return Config{
		Lenses: map[string]string{
			"style":    "style prose",
			"security": "security prose",
		},
		Fans: map[string][]string{
			"standard": {"style", "security"},
			"badlens":  {"style", "ghost"},
			"huge":     huge,
		},
	}
}

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

// TestProfile_Validate table-drives validate over the happy paths and every fail-loud rule.
// The two path-resolution rows also assert, through check, that every path field is rewritten in place to a cleaned absolute path: relative entries are joined onto worktreeRoot per entry, and an already-absolute entry outside worktreeRoot survives unchanged apart from filepath.Clean.
func TestProfile_Validate(t *testing.T) {
	t.Parallel()

	// outsideTarget is an existing absolute path in a directory of its own, outside every subtest's worktree root; subtests only read it.
	elsewhere := t.TempDir()
	writeFixtureFile(t, elsewhere, "outside.txt", "outside content")
	outsideTarget := filepath.Join(elsewhere, "outside.txt")

	tests := []struct {
		name      string
		mutate    func(t *testing.T, root string, p *Profile)
		wantErr   bool
		errSubstr string
		check     func(t *testing.T, root string, got Profile)
	}{
		{
			name:    "valid profile",
			mutate:  func(t *testing.T, root string, p *Profile) {},
			wantErr: false,
		},
		{
			// A relative Fasit entry beside an already-absolute one proves both branches of resolvePath run inside a single field.
			name: "every path field resolves in place",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Fasit.Paths = []string{"fasit.txt", filepath.Join(root, "fasit.txt")}
				writeFixtureFile(t, root, "focus.md", "focus")
				p.FocusDirective = "focus.md"
			},
			check: func(t *testing.T, root string, got Profile) {
				if want := []string{filepath.Join(root, "target.txt")}; diffStrings(got.Target.Paths, want) {
					t.Errorf("Target.Paths = %v; want %v", got.Target.Paths, want)
				}
				if want := []string{filepath.Join(root, "fasit.txt"), filepath.Join(root, "fasit.txt")}; diffStrings(got.Fasit.Paths, want) {
					t.Errorf("Fasit.Paths = %v; want %v", got.Fasit.Paths, want)
				}
				if want := []string{filepath.Join(root, "prior-review.md")}; diffStrings(got.PriorReviews, want) {
					t.Errorf("PriorReviews = %v; want %v", got.PriorReviews, want)
				}
				if want := []string{filepath.Join(root, "prior-fixer.md")}; diffStrings(got.PriorFixerReports, want) {
					t.Errorf("PriorFixerReports = %v; want %v", got.PriorFixerReports, want)
				}
				if want := filepath.Join(root, "focus.md"); got.FocusDirective != want {
					t.Errorf("FocusDirective = %q; want %q", got.FocusDirective, want)
				}
				if want := filepath.Join(root, "review.md"); got.ReviewPath != want {
					t.Errorf("ReviewPath = %q; want %q", got.ReviewPath, want)
				}
				if want := filepath.Join(root, "fixer-report.md"); got.FixerReportPath != want {
					t.Errorf("FixerReportPath = %q; want %q", got.FixerReportPath, want)
				}
			},
		},
		{
			name: "absolute target path outside the worktree is kept",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target.Paths = []string{outsideTarget}
			},
			check: func(t *testing.T, root string, got Profile) {
				if want := []string{outsideTarget}; diffStrings(got.Target.Paths, want) {
					t.Errorf("Target.Paths = %v; want %v", got.Target.Paths, want)
				}
			},
		},
		{
			name: "target directory entry is valid",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target.Paths = []string{"targetdir"}
			},
			wantErr: false,
		},
		{
			name: "target instructions only",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target = FileSet{Instructions: "review the diff against main"}
			},
			wantErr: false,
		},
		{
			name: "fasit instructions only",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Fasit = FileSet{Instructions: "judge against the discussion"}
			},
			wantErr: false,
		},
		{
			name: "target empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target = FileSet{}
			},
			wantErr:   true,
			errSubstr: "profile.Target must set at least one of Paths or Instructions",
		},
		{
			name: "target instructions whitespace only",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target = FileSet{Instructions: "   "}
			},
			wantErr:   true,
			errSubstr: "profile.Target must set at least one of Paths or Instructions",
		},
		{
			name: "fasit empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Fasit = FileSet{}
			},
			wantErr:   true,
			errSubstr: "internal-consistency checking",
		},
		{
			name: "target path missing",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Target.Paths = []string{"does-not-exist.txt"}
			},
			wantErr:   true,
			errSubstr: "profile.Target.Paths entry",
		},
		{
			name: "fasit path missing",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Fasit.Paths = []string{"does-not-exist.txt"}
			},
			wantErr:   true,
			errSubstr: "profile.Fasit.Paths entry",
		},
		{
			name: "prior review path missing",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.PriorReviews = []string{"does-not-exist.md"}
			},
			wantErr:   true,
			errSubstr: "profile.PriorReviews entry",
		},
		{
			name: "prior fixer report path missing",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.PriorFixerReports = []string{"does-not-exist.md"}
			},
			wantErr:   true,
			errSubstr: "profile.PriorFixerReports entry",
		},
		{
			name: "rubric empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Rubric = ""
			},
			wantErr:   true,
			errSubstr: "profile.Rubric must not be empty",
		},
		{
			name: "rubric whitespace only",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.Rubric = "   \n\t "
			},
			wantErr:   true,
			errSubstr: "profile.Rubric must not be empty",
		},
		{
			name: "fixscope empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.FixScope = ""
			},
			wantErr:   true,
			errSubstr: "profile.FixScope must be",
		},
		{
			name: "fixscope invalid",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.FixScope = "markdown"
			},
			wantErr:   true,
			errSubstr: "profile.FixScope must be",
		},
		{
			// Empty ClusterFan (the default) must skip fan resolution
			// entirely — clustering is never on unless a profile names a
			// fan — so it must not error even against a cfg with zero fans
			// configured at all.
			name: "clusterfan empty skips resolution",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ClusterFan = ""
			},
			wantErr: false,
		},
		{
			name: "clusterfan happy path",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ClusterFan = "standard"
			},
			wantErr: false,
		},
		{
			name: "clusterfan unknown fan",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ClusterFan = "missing"
			},
			wantErr:   true,
			errSubstr: "unknown fan",
		},
		{
			name: "clusterfan unknown lens",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ClusterFan = "badlens"
			},
			wantErr:   true,
			errSubstr: "undefined lens",
		},
		{
			name: "clusterfan over cap",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ClusterFan = "huge"
			},
			wantErr:   true,
			errSubstr: "exceeding the maximum",
		},
		{
			name: "focusdirective nonexistent",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.FocusDirective = "no-such-focus.md"
			},
			wantErr:   true,
			errSubstr: "profile.FocusDirective",
		},
		{
			name: "focusdirective existing",
			mutate: func(t *testing.T, root string, p *Profile) {
				writeFixtureFile(t, root, "focus.md", "focus")
				p.FocusDirective = "focus.md"
			},
		},
		{
			name: "reviewpath empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ReviewPath = ""
			},
			wantErr:   true,
			errSubstr: "profile.ReviewPath must not be empty",
		},
		{
			name: "fixerreportpath empty",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.FixerReportPath = ""
			},
			wantErr:   true,
			errSubstr: "profile.FixerReportPath must not be empty",
		},
		{
			// B1: a same-path pair must be rejected — the shuttle file
			// contract's two output-file entries would both be satisfied by
			// one write, silently collapsing the two-artifact contract into
			// one file (proven live).
			name: "reviewpath and fixerreportpath identical (literal)",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.FixerReportPath = p.ReviewPath
			},
			wantErr:   true,
			errSubstr: "must not be the same path",
		},
		{
			// The distinctness check must run on the RESOLVED absolute
			// paths, not the pre-resolution literal strings — an
			// already-absolute FixerReportPath that happens to resolve to
			// the same file as a relative ReviewPath is just as degenerate
			// and must be caught the same way.
			name: "reviewpath and fixerreportpath identical (post-resolution)",
			mutate: func(t *testing.T, root string, p *Profile) {
				p.ReviewPath = "review.md"
				p.FixerReportPath = filepath.Join(root, "review.md")
			},
			wantErr:   true,
			errSubstr: "must not be the same path",
		},
	}

	cfg := testClusterFanConfig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newValidProfileFixture(t)
			tt.mutate(t, root, &p)

			err := p.validate(root, cfg)

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
				// Every validate error carries exactly one "burler: " prefix.
				// A wrapped ResolveFan error that also spells its own
				// "burler: " would double it in the final message — this
				// caught exactly that bug once (N1).
				if n := strings.Count(err.Error(), "burler: "); n != 1 {
					t.Errorf("validate() error = %q; want exactly one %q prefix, found %d", err.Error(), "burler: ", n)
				}
				return
			}

			if err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
			if tt.check != nil {
				tt.check(t, root, p)
			}
		})
	}
}

// TestProfileValidate_ClusterExclude table-drives Profile.ClusterExclude through validate: the no-exclusion happy path, a single-name drop, an absent-name no-op, a duplicate-name no-op, an exclude-everything no-op, the ClusterExclude-without-ClusterFan error, and an empty ClusterFan leaving the lenses nil -- clustering is never on unless a profile names a fan.
//
//testtiming:keep pins the exclusion semantics, the resolved lens order and text and the ClusterExclude-without-ClusterFan error, which the Validate table covering its blocks does not assert
func TestProfileValidate_ClusterExclude(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		clusterFan    string
		excludeLenses []string
		wantErr       bool
		errSubstr     string
		wantNames     []string
	}{
		{
			name:          "no exclusion behaves as today",
			clusterFan:    "standard",
			excludeLenses: nil,
			wantNames:     []string{"style", "security"},
		},
		{
			name:          "exclusion drops the named lens",
			clusterFan:    "standard",
			excludeLenses: []string{"style"},
			wantNames:     []string{"security"},
		},
		{
			name:          "exclusion naming an absent lens is a no-op",
			clusterFan:    "standard",
			excludeLenses: []string{"ghost"},
			wantNames:     []string{"style", "security"},
		},
		{
			name:          "duplicate name is harmless",
			clusterFan:    "standard",
			excludeLenses: []string{"style", "style"},
			wantNames:     []string{"security"},
		},
		{
			name:          "excluding every lens keeps the full fan",
			clusterFan:    "standard",
			excludeLenses: []string{"style", "security"},
			wantNames:     []string{"style", "security"},
		},
		{
			name:          "clusterexclude without clusterfan is an error",
			clusterFan:    "",
			excludeLenses: []string{"style"},
			wantErr:       true,
			errSubstr:     "ClusterExclude",
		},
		{
			name:       "empty clusterfan leaves the lenses nil",
			clusterFan: "",
			wantNames:  nil,
		},
	}

	cfg := testClusterFanConfig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newValidProfileFixture(t)
			p.ClusterFan = tt.clusterFan
			p.ClusterExclude = tt.excludeLenses

			err := p.validate(root, cfg)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() = nil; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validate() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "burler: ") {
					t.Errorf("validate() error = %q; want burler: -prefixed message", err.Error())
				}
				if n := strings.Count(err.Error(), "burler: "); n != 1 {
					t.Errorf("validate() error = %q; want exactly one %q prefix, found %d", err.Error(), "burler: ", n)
				}
				return
			}

			if err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
			if tt.clusterFan == "" && p.clusterLenses != nil {
				t.Errorf("clusterLenses = %+v; want nil for an empty ClusterFan", p.clusterLenses)
			}
			gotNames := make([]string, len(p.clusterLenses))
			for i, lens := range p.clusterLenses {
				gotNames[i] = lens.Name
				if want := cfg.Lenses[lens.Name]; lens.Text != want {
					t.Errorf("clusterLenses[%d].Text = %q; want the configured text %q", i, lens.Text, want)
				}
			}
			if diffStrings(gotNames, tt.wantNames) {
				t.Errorf("clusterLenses names = %v; want %v", gotNames, tt.wantNames)
			}
		})
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
