package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// bouncerFixtureStampHash is the fake but well-formed 64-lowercase-hex sha256 newBouncerStencilsFixture
// stamps every fixture file with, via stencilstore.ApplyStamp -- realistic in shape, never checked
// for correctness by anything this fixture feeds.
const bouncerFixtureStampHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// newBouncerStencilsFixture builds a stencils fixture directory under t.TempDir(), writing each
// named stencil at <dir>/bouncer/<name>.md, matching stencilstore.RelPath's family-from-first-token
// derivation, and giving each file a realistic leading <!-- lyx-stencil: sha256=... --> stamp
// banner via stencilstore.ApplyStamp -- which merges into an existing leading comment rather than
// nesting a second one, so a body already carrying its own leading comment (as the shipped bouncer
// templates do) still ends up with exactly one banner for stencil.Fill's leading-comment strip to
// remove. Batches 3 and 4's later test files reuse this helper.
func newBouncerStencilsFixture(t *testing.T, stencils map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, body := range stencils {
		relPath := stencilstore.RelPath(name)
		absPath := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(absPath), err)
		}
		content := stencilstore.ApplyStamp([]byte(body), bouncerFixtureStampHash)
		if err := os.WriteFile(absPath, content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", absPath, err)
		}
	}
	return dir
}

// validBouncerConfig returns a BouncerConfig satisfying every NewBouncer validation rule,
// resolved against a temp run dir and a stencils fixture seeded with rubricName.
func validBouncerConfig(t *testing.T, rubricName string) BouncerConfig {
	t.Helper()

	runDir := t.TempDir()
	stencilsDir := newBouncerStencilsFixture(t, map[string]string{
		rubricName: "# Rubric\n\nBe thorough.\n",
	})
	return BouncerConfig{
		Name:          "gate",
		RunDir:        runDir,
		ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
		ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
		StencilsDir:   stencilsDir,
		RubricStencil: rubricName,
		Shuttle:       &fakeShuttle{},
	}
}

func TestNewBouncer_ValidationRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *BouncerConfig)
		wantErr string
	}{
		{
			name:    "EmptyName",
			mutate:  func(cfg *BouncerConfig) { cfg.Name = "" },
			wantErr: "Name",
		},
		{
			name:    "EmptyRunDir",
			mutate:  func(cfg *BouncerConfig) { cfg.RunDir = "" },
			wantErr: "RunDir",
		},
		{
			name:    "RelativeRunDir",
			mutate:  func(cfg *BouncerConfig) { cfg.RunDir = "relative/run" },
			wantErr: "RunDir",
		},
		{
			name:    "EmptyArtifactPaths",
			mutate:  func(cfg *BouncerConfig) { cfg.ArtifactPaths = nil },
			wantErr: "ArtifactPaths",
		},
		{
			name:    "RelativeArtifactPathsEntry",
			mutate:  func(cfg *BouncerConfig) { cfg.ArtifactPaths = []string{"relative/artifact.md"} },
			wantErr: "ArtifactPaths",
		},
		{
			name:    "NilReportName",
			mutate:  func(cfg *BouncerConfig) { cfg.ReportName = nil },
			wantErr: "ReportName",
		},
		{
			name:    "EmptyStencilsDir",
			mutate:  func(cfg *BouncerConfig) { cfg.StencilsDir = "" },
			wantErr: "StencilsDir",
		},
		{
			name:    "RelativeStencilsDir",
			mutate:  func(cfg *BouncerConfig) { cfg.StencilsDir = "relative/stencils" },
			wantErr: "StencilsDir",
		},
		{
			name:    "EmptyRubricStencil",
			mutate:  func(cfg *BouncerConfig) { cfg.RubricStencil = "" },
			wantErr: "RubricStencil",
		},
		{
			name:    "NilShuttle",
			mutate:  func(cfg *BouncerConfig) { cfg.Shuttle = nil },
			wantErr: "Shuttle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBouncerConfig(t, "bouncer-template-rubric-"+tt.name)
			tt.mutate(&cfg)

			_, err := NewBouncer(cfg)
			if err == nil {
				t.Fatalf("NewBouncer(%+v) error = nil; want non-nil naming %q", cfg, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NewBouncer(...) error = %q; want it to name %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewBouncer_EmptyModelEffortVersionAccepted(t *testing.T) {
	cfg := validBouncerConfig(t, "bouncer-template-rubric-empty-triple")
	cfg.Model = ""
	cfg.Effort = ""
	cfg.Version = ""

	if _, err := NewBouncer(cfg); err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil for empty Model/Effort/Version", err)
	}
}

func TestNewBouncer_NilNowDefaultsToNonNilClock(t *testing.T) {
	cfg := validBouncerConfig(t, "bouncer-template-rubric-nil-now")
	cfg.Now = nil

	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	if b.cfg.Now == nil {
		t.Error("NewBouncer(...).cfg.Now = nil; want a non-nil clock")
	}
}

func TestNewBouncer_ArtifactPathNeedNotExist(t *testing.T) {
	cfg := validBouncerConfig(t, "bouncer-template-rubric-nonexistent-artifact")
	cfg.ArtifactPaths = []string{filepath.Join(cfg.RunDir, "not-yet-written.md")}

	if _, err := NewBouncer(cfg); err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil for a not-yet-existing artifact path", err)
	}
}

func TestNewBouncer_RubricProbe(t *testing.T) {
	t.Run("Absent", func(t *testing.T) {
		cfg := validBouncerConfig(t, "bouncer-template-rubric-present")
		cfg.RubricStencil = "bouncer-template-rubric-absent"

		_, err := NewBouncer(cfg)
		if err == nil {
			t.Fatal("NewBouncer(...) error = nil; want non-nil for an unreadable RubricStencil")
		}
		if !strings.Contains(err.Error(), "RubricStencil") {
			t.Errorf("NewBouncer(...) error = %q; want it to name RubricStencil", err.Error())
		}
	})

	t.Run("Readable", func(t *testing.T) {
		cfg := validBouncerConfig(t, "bouncer-template-rubric-readable")

		if _, err := NewBouncer(cfg); err != nil {
			t.Fatalf("NewBouncer(...) error = %v; want nil for a readable RubricStencil", err)
		}
	})
}
