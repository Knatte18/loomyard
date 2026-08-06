// header_test.go covers HeaderText and ValidateHeader hermetically: an
// Engine built from Config/*lyxcwd.Location struct literals (no
// lyxcwd.Resolve, no tmux spawn), per the Test Tier Purity Invariant.

package reedengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newHeaderTestEngine builds a test Engine with the given header template.
func newHeaderTestEngine(template string) *Engine {
	layout := &lyxcwd.Location{RepoName: "test-repo", HubPath: "test-hub"}
	cfg := Config{
		Header: HeaderConfig{Template: template},
	}
	return New(cfg, layout)
}

func TestHeaderText_EmptyTemplateRendersEmbeddedDefault(t *testing.T) {
	e := newHeaderTestEngine("")

	got, err := e.HeaderText()
	if err != nil {
		t.Fatalf("HeaderText() unexpected error: %v", err)
	}

	want := "hub: " + e.layout.HubPath
	if strings.TrimSpace(got) != want {
		t.Errorf("HeaderText() = %q; want %q", strings.TrimSpace(got), want)
	}
}

func TestHeaderText_ConfiguredTemplateRendersFromConfig(t *testing.T) {
	e := newHeaderTestEngine("repo: {{.repo}}")

	got, err := e.HeaderText()
	if err != nil {
		t.Fatalf("HeaderText() unexpected error: %v", err)
	}

	want := "repo: " + e.layout.RepoName
	if got != want {
		t.Errorf("HeaderText() = %q; want %q", got, want)
	}
}

func TestValidateHeader_UnknownTopLevelTokenErrors(t *testing.T) {
	e := newHeaderTestEngine("{{.slug}}")

	if err := e.ValidateHeader(); err == nil {
		t.Error("ValidateHeader() = nil; want an error for an unknown top-level token")
	}
}

func TestValidateHeader_GoodTemplateReturnsNil(t *testing.T) {
	e := newHeaderTestEngine("repo: {{.repo}}")

	if err := e.ValidateHeader(); err != nil {
		t.Errorf("ValidateHeader() = %v; want nil", err)
	}
}
