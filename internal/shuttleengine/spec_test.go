// spec_test.go verifies Spec.validate's normalization and error paths:
// mandatory Prompt/OutputFiles, relative-to-absolute resolution for output
// files, and the Timeout/Display.Anchor defaulting rules.

package shuttleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func TestSpec_Validate_EmptyPrompt(t *testing.T) {
	s := &Spec{OutputFiles: []string{"out.md"}}
	err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 30})
	if err == nil {
		t.Fatal("validate() = nil, want error for empty Prompt")
	}
}

func TestSpec_Validate_EmptyOutputFiles(t *testing.T) {
	s := &Spec{Prompt: "do the thing"}
	err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 30})
	if err == nil {
		t.Fatal("validate() = nil, want error for empty OutputFiles")
	}
}

func TestSpec_Validate_RelativeOutputFilesResolveToAbsolute(t *testing.T) {
	worktreeRoot := `C:\worktree`
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md", "sub/report.json"}}
	if err := s.validate(worktreeRoot, Config{RunTimeoutMin: 30}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}

	want := []string{
		filepath.Clean(filepath.Join(worktreeRoot, "out.md")),
		filepath.Clean(filepath.Join(worktreeRoot, "sub/report.json")),
	}
	for i, got := range s.OutputFiles {
		if got != want[i] {
			t.Errorf("OutputFiles[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestSpec_Validate_AbsoluteOutputFilesPassThroughVerbatim(t *testing.T) {
	abs := `D:\elsewhere\out.md`
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{abs}}
	if err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 30}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if s.OutputFiles[0] != abs {
		t.Errorf("OutputFiles[0] = %q, want %q (absolute passthrough)", s.OutputFiles[0], abs)
	}
}

func TestSpec_Validate_PreExistingOutputFileRejected(t *testing.T) {
	// A pre-existing output file would satisfy the file contract on the
	// very first turn end, silently classifying an asking run as done
	// (proven live) — validate must reject it loudly instead.
	worktreeRoot := t.TempDir()
	stale := filepath.Join(worktreeRoot, "out.md")
	if err := os.WriteFile(stale, []byte("stale artifact"), 0o644); err != nil {
		t.Fatalf("seed stale output file: %v", err)
	}

	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}}
	err := s.validate(worktreeRoot, Config{RunTimeoutMin: 30})
	if err == nil {
		t.Fatal("validate() = nil, want error for pre-existing output file")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("validate() error = %q, want it to name the pre-existing file", err)
	}
}

func TestSpec_Validate_TimeoutDefaultsFromConfig(t *testing.T) {
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}}
	if err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 45}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	want := 45 * time.Minute
	if s.Timeout != want {
		t.Errorf("Timeout = %v, want %v", s.Timeout, want)
	}
}

func TestSpec_Validate_TimeoutPassThroughWhenSet(t *testing.T) {
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Timeout: 5 * time.Minute}
	if err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 45}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if s.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want unchanged 5m", s.Timeout)
	}
}

func TestSpec_Validate_AnchorDefaultsToBelowParent(t *testing.T) {
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}}
	if err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 30}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if s.Display.Anchor != render.AnchorBelowParent {
		t.Errorf("Display.Anchor = %q, want %q", s.Display.Anchor, render.AnchorBelowParent)
	}
}

func TestSpec_Validate_AnchorPassThroughWhenSet(t *testing.T) {
	s := &Spec{Prompt: "do the thing", OutputFiles: []string{"out.md"}, Display: render.Display{Anchor: render.AnchorTop}}
	if err := s.validate(`C:\worktree`, Config{RunTimeoutMin: 30}); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if s.Display.Anchor != render.AnchorTop {
		t.Errorf("Display.Anchor = %q, want unchanged %q", s.Display.Anchor, render.AnchorTop)
	}
}
