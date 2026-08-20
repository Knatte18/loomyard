// identity_test.go tables ProfileHash, DeriveRunID, and ValidRunID — perch's own block-identity
// derivation (identity.go), extracted from state_test.go (now internal/treadleengine/state_test.go)
// when the round-state machinery it exercised moved to treadleengine;
// these three functions and sanitizeSlug stayed perch-side.

package perchengine

import (
	"path/filepath"
	"testing"
)

// TestScratchDir proves ScratchDir is RunsDir's mirrored sibling under
// .lyx: the same anchor and the same perchDirName segment, differing only
// in swapping _lyx for .lyx — for both a plain (unnested) anchor path and a
// nested-subpath anchor path.
func TestScratchDir(t *testing.T) {
	tests := []struct {
		name       string
		anchorPath string
	}{
		{
			name:       "plain anchor path",
			anchorPath: t.TempDir(),
		},
		{
			name:       "nested-subpath anchor path",
			anchorPath: filepath.Join(t.TempDir(), "nested", "sub"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(tt.anchorPath, ".lyx", "perch")
			got := ScratchDir(tt.anchorPath)
			if got != want {
				t.Errorf("ScratchDir(%q) = %q; want %q", tt.anchorPath, got, want)
			}

			runsDir := RunsDir(tt.anchorPath)
			if got == runsDir {
				t.Errorf("ScratchDir(%q) = %q; want it to differ from RunsDir(%q) = %q", tt.anchorPath, got, tt.anchorPath, runsDir)
			}
			// The two must differ in exactly the _lyx/.lyx segment: swapping
			// it in RunsDir's own result must reproduce ScratchDir's.
			wantFromRunsDir := filepath.Join(filepath.Dir(filepath.Dir(runsDir)), ".lyx", "perch")
			if got != wantFromRunsDir {
				t.Errorf("ScratchDir(%q) = %q; want it to differ from RunsDir(%q) = %q only in the _lyx/.lyx segment", tt.anchorPath, got, tt.anchorPath, runsDir)
			}
		})
	}
}

func TestProfileHash(t *testing.T) {
	p1 := Profile{Rubric: "a", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}
	p2 := Profile{Rubric: "a", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}
	p3 := Profile{Rubric: "b", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}

	h1, err := ProfileHash(p1)
	if err != nil {
		t.Fatalf("ProfileHash(p1) = %v; want nil", err)
	}
	h2, err := ProfileHash(p2)
	if err != nil {
		t.Fatalf("ProfileHash(p2) = %v; want nil", err)
	}
	h3, err := ProfileHash(p3)
	if err != nil {
		t.Fatalf("ProfileHash(p3) = %v; want nil", err)
	}

	if h1 != h2 {
		t.Errorf("ProfileHash(p1) = %q; ProfileHash(p2) = %q; want equal for identical profiles", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("ProfileHash(p1) = ProfileHash(p3) = %q; want different for differing profiles", h1)
	}
	if len(h1) != 64 {
		t.Errorf("len(ProfileHash(p1)) = %d; want 64 (sha256 hex)", len(h1))
	}
}

func TestDeriveRunID(t *testing.T) {
	tests := []struct {
		name        string
		profilePath string
		hash        string
		want        string
	}{
		{
			name:        "simple basename",
			profilePath: filepath.Join("profiles", "code-review.yaml"),
			hash:        "abcdef0123456789",
			want:        "code-review-abcdef01",
		},
		{
			name:        "spaces and mixed case sanitize to dashes",
			profilePath: filepath.Join("profiles", "My Profile.yml"),
			hash:        "0011223344556677",
			want:        "my-profile-00112233",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveRunID(tt.profilePath, tt.hash)
			if got != tt.want {
				t.Errorf("DeriveRunID(%q, %q) = %q; want %q", tt.profilePath, tt.hash, got, tt.want)
			}
		})
	}
}

// TestValidRunID proves the exact shape both perch CLI verbs must enforce on an explicit --run-id
// before joining it into a run-dir path: every id DeriveRunID could plausibly produce passes, while
// anything carrying a path separator, a leading/trailing dash, or other punctuation — the class of
// value that would escape RunsDir via filepath.Join — fails.
func TestValidRunID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"derived-shape id", "code-review-abcdef01", true},
		{"single word", "myrun", true},
		{"empty", "", false},
		{"parent-dir traversal", "../elsewhere", false},
		{"nested parent-dir traversal", "../../perchwork", false},
		{"absolute path", "/etc/passwd", false},
		{"windows path separator", `a\b`, false},
		{"leading dash", "-abc", false},
		{"trailing dash", "abc-", false},
		{"uppercase", "MyRun", false},
		{"space", "my run", false},
		{"dot", "my.run", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidRunID(tt.id); got != tt.want {
				t.Errorf("ValidRunID(%q) = %v; want %v", tt.id, got, tt.want)
			}
		})
	}
}
