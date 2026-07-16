// header_test.go covers HeaderText and ValidateHeader hermetically: an
// Engine built from Config/*hubgeometry.Layout struct literals (no
// hubgeometry.Resolve, no tmux spawn), per the Test Tier Purity Invariant.

package muxengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newHeaderTestEngine builds an Engine rooted at a fixed Layout/Config pair
// for header-pipeline tests, with cfg.Header.Template set to template so
// each subtest can exercise either the embedded default (empty template) or
// a config override without touching disk or spawning tmux.
func newHeaderTestEngine(template string) *Engine {
	layout := &hubgeometry.Layout{
		Hub:  "test-hub",
		Repo: "test-repo",
	}
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

	want := "hub: " + e.layout.Hub
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

	want := "repo: " + e.layout.Repo
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
