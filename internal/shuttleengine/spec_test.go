// spec_test.go verifies Spec.validate's normalization and error paths: mandatory
// Prompt/OutputFiles, relative-to-absolute resolution for output files, the Timeout
// defaulting/negative-rejection rules, and the Display.Anchor default.

package shuttleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestSpec_Validate drives Spec.validate through its rejections, its defaults and the fields it
// must leave alone. Effort, Version and NameOverride are provider or engine
// vocabulary validated elsewhere, so validate must neither default nor reject them: a later "tidy
// up the validator" change must not quietly start doing either.
func TestSpec_Validate(t *testing.T) {
	t.Parallel()

	const worktreeRoot = `C:\worktree`
	// A pre-existing output file would satisfy the file contract on the very first turn end,
	// silently classifying an asking run as done (proven live), so validate must reject it loudly.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.md"), []byte("stale artifact"), 0o644); err != nil {
		t.Fatalf("seed stale output file: %v", err)
	}
	// An OS-absolute OutputFiles entry must pass through unchanged, not be re-rooted at the
	// worktree. t.TempDir() is absolute on any host; the file is fresh because validate rejects
	// pre-existing output files.
	abs := filepath.Join(dir, "fresh.md")

	tests := []struct {
		name         string
		spec         Spec
		root         string
		runTimeout   int
		wantErr      bool
		wantErrIn    string
		wantOutputIn []string
		check        func(t *testing.T, s *Spec)
	}{
		{name: "empty prompt", spec: Spec{OutputFiles: []string{"out.md"}}, wantErr: true},
		{name: "empty output files", spec: Spec{Prompt: "do the thing"}, wantErr: true},
		{
			name: "relative output files resolve to absolute",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md", "sub/report.json"}},
			wantOutputIn: []string{
				filepath.Clean(filepath.Join(worktreeRoot, "out.md")),
				filepath.Clean(filepath.Join(worktreeRoot, "sub/report.json")),
			},
		},
		{
			name:         "absolute output files pass through verbatim",
			spec:         Spec{Prompt: "do the thing", OutputFiles: []string{abs}},
			wantOutputIn: []string{abs},
		},
		{
			name:      "pre-existing output file rejected",
			spec:      Spec{Prompt: "do the thing", OutputFiles: []string{"stale.md"}},
			root:      dir,
			wantErr:   true,
			wantErrIn: "already exists",
		},
		{
			name:       "timeout defaults from config",
			spec:       Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}},
			runTimeout: 45,
			check: func(t *testing.T, s *Spec) {
				if want := 45 * time.Minute; s.Timeout != want {
					t.Errorf("Timeout = %v, want %v", s.Timeout, want)
				}
			},
		},
		{
			name:       "timeout passes through when set",
			spec:       Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Timeout: 5 * time.Minute},
			runTimeout: 45,
			check: func(t *testing.T, s *Spec) {
				if s.Timeout != 5*time.Minute {
					t.Errorf("Timeout = %v, want unchanged 5m", s.Timeout)
				}
			},
		},
		{
			// An instant-timeout run would leave stray live state.
			name:      "negative timeout rejected",
			spec:      Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Timeout: -5 * time.Second},
			wantErr:   true,
			wantErrIn: "must not be negative",
		},
		{
			name: "anchor defaults to below parent",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}},
			check: func(t *testing.T, s *Spec) {
				if s.Display.Anchor != render.AnchorBelowParent {
					t.Errorf("Display.Anchor = %q, want %q", s.Display.Anchor, render.AnchorBelowParent)
				}
			},
		},
		{
			name: "anchor passes through when set",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Display: render.Display{Anchor: render.AnchorBelowParent}},
			check: func(t *testing.T, s *Spec) {
				if s.Display.Anchor != render.AnchorBelowParent {
					t.Errorf("Display.Anchor = %q, want unchanged %q", s.Display.Anchor, render.AnchorBelowParent)
				}
			},
		},
		{
			name: "empty effort stays empty",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}},
			check: func(t *testing.T, s *Spec) {
				if s.Effort != "" {
					t.Errorf("Effort = %q, want unchanged empty string", s.Effort)
				}
			},
		},
		{
			name: "nonsense effort is neither rejected nor changed",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"other.md"}, Effort: "not-a-real-effort-value"},
			check: func(t *testing.T, s *Spec) {
				if s.Effort != "not-a-real-effort-value" {
					t.Errorf("Effort = %q, want unchanged %q", s.Effort, "not-a-real-effort-value")
				}
			},
		},
		{
			name: "empty version stays empty",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}},
			check: func(t *testing.T, s *Spec) {
				if s.Version != "" {
					t.Errorf("Version = %q, want unchanged empty string", s.Version)
				}
			},
		},
		{
			name: "version is neither rejected nor changed",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"other.md"}, Version: "4.5"},
			check: func(t *testing.T, s *Spec) {
				if s.Version != "4.5" {
					t.Errorf("Version = %q, want unchanged %q", s.Version, "4.5")
				}
			},
		},
		{
			name: "nonsense version is neither rejected nor changed",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"third.md"}, Version: "weird"},
			check: func(t *testing.T, s *Spec) {
				if s.Version != "weird" {
					t.Errorf("Version = %q, want unchanged %q", s.Version, "weird")
				}
			},
		},
		{
			name: "empty name override stays empty",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}},
			check: func(t *testing.T, s *Spec) {
				if s.NameOverride != "" {
					t.Errorf("NameOverride = %q, want unchanged empty string", s.NameOverride)
				}
			},
		},
		{
			name: "name override is neither rejected nor changed",
			spec: Spec{Prompt: "do the thing", OutputFiles: []string{"other.md"}, NameOverride: "driver"},
			check: func(t *testing.T, s *Spec) {
				if s.NameOverride != "driver" {
					t.Errorf("NameOverride = %q, want unchanged %q", s.NameOverride, "driver")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := tt.root
			if root == "" {
				root = worktreeRoot
			}
			runTimeout := tt.runTimeout
			if runTimeout == 0 {
				runTimeout = 30
			}
			s := tt.spec
			err := s.validate(root, Config{RunTimeoutMin: runTimeout})
			if tt.wantErr {
				if err == nil {
					t.Fatal("validate() = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Errorf("validate() error = %q, want it to contain %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() error: %v", err)
			}
			for i, want := range tt.wantOutputIn {
				if s.OutputFiles[i] != want {
					t.Errorf("OutputFiles[%d] = %q, want %q", i, s.OutputFiles[i], want)
				}
			}
			if tt.check != nil {
				tt.check(t, &s)
			}
		})
	}
}
