// rubric_test.go covers ReadRubric directly: the fill, the strip, the markerless pass-through, and
// the required-marker error, exercised against a realistically stamped fixture file rather than a
// bare body.

package shedadapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// writeStampedRubric writes body into dir at name's stencilstore path, stamped with body's own real
// BodyHash via ApplyStamp -- a realistically stamped fixture, not a bare body, since several
// assertions in this file are about the stamp banner surviving (or not surviving) the read.
func writeStampedRubric(t *testing.T, dir, name, body string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	stamped := stencilstore.ApplyStamp([]byte(body), stencilstore.BodyHash([]byte(body)))
	if err := os.WriteFile(path, stamped, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestReadRubric covers the fill, the strip and the errors; "{dir}" in want stands for the
// stencils dir the row was read from. An exact want also pins the stamp banner stripped.
//
//testtiming:keep pins the rubric fill and strip: both markers substituted, the stamp banner removed, a markerless rubric unchanged, and the errors for an empty specs dir or an unreadable rubric
func TestReadRubric(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rubric   string
		body     string
		specsDir string
		// publishFailure is the note ReadRubric fills the publish_failure marker from.
		publishFailure string
		want           string
		wantErr        string
	}{
		{
			name:           "renders the publish failure note",
			rubric:         "bouncer-rubric-test",
			body:           "# Rubric\n\nFailure: {{.publish_failure}}\n",
			specsDir:       "/abs/specs",
			publishFailure: "Publish failed on the plan's verify.",
			want:           "# Rubric\n\nFailure: Publish failed on the plan's verify.\n",
		},
		{
			name:     "a blank note renders none",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nFailure: {{.publish_failure}}\n",
			specsDir: "/abs/specs",
			want:     "# Rubric\n\nFailure: none\n",
		},
		{
			name:     "substitutes the specs dir",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nSee {{.specs_dir}} for the format contract.\n",
			specsDir: "/abs/specs",
			want:     "# Rubric\n\nSee /abs/specs for the format contract.\n",
		},
		{
			name:     "substitutes the stencils dir",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nStay symmetric with {{.stencils_dir}}/loom/x.md.\n",
			specsDir: "/abs/specs",
			want:     "# Rubric\n\nStay symmetric with {dir}/loom/x.md.\n",
		},
		{
			name:     "substitutes both markers",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nSpecs {{.specs_dir}}, stencils {{.stencils_dir}}.\n",
			specsDir: "/abs/specs",
			want:     "# Rubric\n\nSpecs /abs/specs, stencils {dir}.\n",
		},
		{
			name:     "markerless rubric renders unchanged",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nBe thorough. No markers here.\n",
			specsDir: "/abs/specs",
			want:     "# Rubric\n\nBe thorough. No markers here.\n",
		},
		{
			name:     "empty specs dir is an error",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nSee {{.specs_dir}}.\n",
			specsDir: "",
			wantErr:  "specs_dir",
		},
		{
			name:     "whitespace-only specs dir is an error",
			rubric:   "bouncer-rubric-test",
			body:     "# Rubric\n\nSee {{.specs_dir}}.\n",
			specsDir: "   ",
			wantErr:  "specs_dir",
		},
		{
			name:     "unreadable rubric is an error naming it",
			rubric:   "bouncer-rubric-missing",
			specsDir: "/abs/specs",
			wantErr:  "bouncer-rubric-missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.body != "" {
				writeStampedRubric(t, dir, tt.rubric, tt.body)
			}

			got, err := ReadRubric(dir, tt.rubric, tt.specsDir, tt.publishFailure)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ReadRubric() = nil error; want one containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ReadRubric() error = %q; want it to contain %q", err.Error(), tt.wantErr)
				}
				if got != "" {
					t.Errorf("ReadRubric() = %q; want empty result on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadRubric() = %v; want nil", err)
			}
			if want := strings.ReplaceAll(tt.want, "{dir}", dir); got != want {
				t.Errorf("ReadRubric() = %q; want %q", got, want)
			}
		})
	}
}

//testtiming:keep pins that an empty stencils dir is an error naming stencils_dir, which needs the cwd changed and so cannot join the parallel table
func TestReadRubric_EmptyStencilsDirIsAnError(t *testing.T) {
	// An empty stencilsDir makes the read cwd-relative, so the fixture lives in the cwd;
	// without that the read fails and the test would pass without ever reaching the fill.
	t.Chdir(t.TempDir())
	writeStampedRubric(t, "", "bouncer-rubric-test", "# Rubric\n\nSee {{.stencils_dir}}.\n")

	got, err := ReadRubric("", "bouncer-rubric-test", "/abs/specs", "")
	if err == nil {
		t.Fatal("ReadRubric(stencilsDir=\"\") = nil error; want non-nil")
	}
	if !strings.Contains(err.Error(), "stencils_dir") {
		t.Errorf("ReadRubric() error = %v; want it to name stencils_dir", err)
	}
	if got != "" {
		t.Errorf("ReadRubric() = %q; want empty result on error", got)
	}
}
