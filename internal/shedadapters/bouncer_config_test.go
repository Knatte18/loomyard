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

//testtiming:keep pins the config shapes NewBouncer accepts: an empty Model, Effort and Version, a nil clock that defaults, and an artifact path that does not exist yet
func TestNewBouncer_AcceptedConfigs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(cfg *BouncerConfig)
	}{
		{
			name: "empty Model, Effort and Version",
			mutate: func(cfg *BouncerConfig) {
				cfg.Model = ""
				cfg.Effort = ""
				cfg.Version = ""
			},
		},
		{
			name:   "a nil Now defaults to a non-nil clock",
			mutate: func(cfg *BouncerConfig) { cfg.Now = nil },
		},
		{
			name: "an artifact path that does not exist yet",
			mutate: func(cfg *BouncerConfig) {
				cfg.ArtifactPaths = []string{filepath.Join(cfg.RunDir, "not-yet-written.md")}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := newBouncerFixture(t, withBareConfig(), withRubricName("bouncer-template-rubric-accepted")).Config
			tt.mutate(&cfg)

			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}
			if b.cfg.Now == nil {
				t.Error("NewBouncer(...).cfg.Now = nil; want a non-nil clock")
			}
		})
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
