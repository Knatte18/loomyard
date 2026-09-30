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

func TestReadRubric_SubstitutesTheSpecsDir(t *testing.T) {
	dir := t.TempDir()
	writeStampedRubric(t, dir, "bouncer-rubric-test", "# Rubric\n\nSee {{.specs_dir}} for the format contract.\n")

	got, err := ReadRubric(dir, "bouncer-rubric-test", "/abs/specs")
	if err != nil {
		t.Fatalf("ReadRubric() = %v; want nil", err)
	}
	if !strings.Contains(got, "/abs/specs") {
		t.Errorf("ReadRubric() = %q; want it to contain the told specs dir", got)
	}
	if strings.Contains(got, "{{.") {
		t.Errorf("ReadRubric() = %q; want no remaining {{. marker", got)
	}
}

func TestReadRubric_StripsTheStampBanner(t *testing.T) {
	dir := t.TempDir()
	body := "# Rubric\n\nBe thorough about {{.specs_dir}}.\n"
	writeStampedRubric(t, dir, "bouncer-rubric-test", body)

	got, err := ReadRubric(dir, "bouncer-rubric-test", "/abs/specs")
	if err != nil {
		t.Fatalf("ReadRubric() = %v; want nil", err)
	}
	if strings.Contains(got, stencilstore.StampPrefix) {
		t.Errorf("ReadRubric() = %q; want no stamp prefix in the result", got)
	}
	hash := stencilstore.BodyHash([]byte(body))
	stampLine := stencilstore.StampPrefix + hash + stencilstore.StampSuffix
	if strings.Contains(got, stampLine) {
		t.Errorf("ReadRubric() = %q; want no stamp line %q in the result", got, stampLine)
	}
}

func TestReadRubric_MarkerlessRubricRendersUnchanged(t *testing.T) {
	dir := t.TempDir()
	body := "# Rubric\n\nBe thorough. No markers here.\n"
	writeStampedRubric(t, dir, "bouncer-rubric-test", body)

	got, err := ReadRubric(dir, "bouncer-rubric-test", "/abs/specs")
	if err != nil {
		t.Fatalf("ReadRubric() = %v; want nil", err)
	}
	if got != body {
		t.Errorf("ReadRubric() = %q; want exactly the stripped body %q", got, body)
	}
}

func TestReadRubric_EmptySpecsDirIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeStampedRubric(t, dir, "bouncer-rubric-test", "# Rubric\n\nSee {{.specs_dir}}.\n")

	tests := []struct {
		name     string
		specsDir string
	}{
		{"Empty", ""},
		{"WhitespaceOnly", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadRubric(dir, "bouncer-rubric-test", tt.specsDir)
			if err == nil {
				t.Fatalf("ReadRubric(specsDir=%q) = nil error; want non-nil", tt.specsDir)
			}
			if got != "" {
				t.Errorf("ReadRubric(specsDir=%q) = %q; want empty result on error", tt.specsDir, got)
			}
		})
	}
}

func TestReadRubric_UnreadableRubricIsAnError(t *testing.T) {
	dir := t.TempDir()

	got, err := ReadRubric(dir, "bouncer-rubric-missing", "/abs/specs")
	if err == nil {
		t.Fatalf("ReadRubric() = nil error; want non-nil")
	}
	if !strings.Contains(err.Error(), "bouncer-rubric-missing") {
		t.Errorf("ReadRubric() error = %q; want it to name the rubric %q", err.Error(), "bouncer-rubric-missing")
	}
	if got != "" {
		t.Errorf("ReadRubric() = %q; want empty result on error", got)
	}
}

func TestReadRubric_SubstitutesTheStencilsDir(t *testing.T) {
	dir := t.TempDir()
	writeStampedRubric(t, dir, "bouncer-rubric-test", "# Rubric\n\nStay symmetric with {{.stencils_dir}}/loom/x.md.\n")

	got, err := ReadRubric(dir, "bouncer-rubric-test", "/abs/specs")
	if err != nil {
		t.Fatalf("ReadRubric() = %v; want nil", err)
	}
	if !strings.Contains(got, dir+"/loom/x.md") {
		t.Errorf("ReadRubric() = %q; want it to contain the told stencils dir %q", got, dir)
	}
	if strings.Contains(got, "{{.") {
		t.Errorf("ReadRubric() = %q; want no remaining {{. marker", got)
	}
}

func TestReadRubric_SubstitutesBothMarkers(t *testing.T) {
	dir := t.TempDir()
	writeStampedRubric(t, dir, "bouncer-rubric-test", "# Rubric\n\nSpecs {{.specs_dir}}, stencils {{.stencils_dir}}.\n")

	got, err := ReadRubric(dir, "bouncer-rubric-test", "/abs/specs")
	if err != nil {
		t.Fatalf("ReadRubric() = %v; want nil", err)
	}
	if !strings.Contains(got, "/abs/specs") || !strings.Contains(got, dir) {
		t.Errorf("ReadRubric() = %q; want both the specs dir and the stencils dir %q", got, dir)
	}
	if strings.Contains(got, "{{.") {
		t.Errorf("ReadRubric() = %q; want no remaining {{. marker", got)
	}
}

func TestReadRubric_EmptyStencilsDirIsAnError(t *testing.T) {
	// An empty stencilsDir makes the read cwd-relative, so the fixture lives in the cwd;
	// without that the read fails and the test would pass without ever reaching the fill.
	t.Chdir(t.TempDir())
	writeStampedRubric(t, "", "bouncer-rubric-test", "# Rubric\n\nSee {{.stencils_dir}}.\n")

	got, err := ReadRubric("", "bouncer-rubric-test", "/abs/specs")
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
