package shedadapters

import (
	"path/filepath"
	"strings"
	"testing"
)

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
			cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-"+tt.name)).Config
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
	cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-empty-triple")).Config
	cfg.Model = ""
	cfg.Effort = ""
	cfg.Version = ""

	if _, err := NewBouncer(cfg); err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil for empty Model/Effort/Version", err)
	}
}

func TestNewBouncer_NilNowDefaultsToNonNilClock(t *testing.T) {
	cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-nil-now")).Config
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
	cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-nonexistent-artifact")).Config
	cfg.ArtifactPaths = []string{filepath.Join(cfg.RunDir, "not-yet-written.md")}

	if _, err := NewBouncer(cfg); err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil for a not-yet-existing artifact path", err)
	}
}

func TestNewBouncer_RubricProbe(t *testing.T) {
	t.Run("Absent", func(t *testing.T) {
		cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-present")).Config
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
		cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-readable")).Config

		if _, err := NewBouncer(cfg); err != nil {
			t.Fatalf("NewBouncer(...) error = %v; want nil for a readable RubricStencil", err)
		}
	})
}
