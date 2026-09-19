// statusline_test.go covers StatusLineText and ValidateStatusLine hermetically: an Engine built from
// Config/Geometry struct literals (no lyxcwd.Resolve, no tmux spawn), per the Test Tier
// Purity Invariant.

package reedengine

import (
	"strings"
	"testing"
)

// newStatusLineTestEngine builds a test Engine with the given status-line template.
func newStatusLineTestEngine(template string) *Engine {
	geom := Geometry{RepoName: "test-repo", WorktreeName: "test-worktree", HubPath: "test-hub"}
	cfg := Config{
		StatusLine: StatusLineConfig{Template: template},
	}
	return New(cfg, geom)
}

func TestStatusLineText_EmptyTemplateRendersEmbeddedDefault(t *testing.T) {
	e := newStatusLineTestEngine("")

	got, err := e.StatusLineText()
	if err != nil {
		t.Fatalf("StatusLineText() unexpected error: %v", err)
	}

	want := e.geom.RepoName + "/" + e.geom.WorktreeName + " · " + e.geom.HubPath
	if strings.TrimSpace(got) != want {
		t.Errorf("StatusLineText() = %q; want %q", strings.TrimSpace(got), want)
	}
}

func TestStatusLineText_ConfiguredTemplateRendersFromConfig(t *testing.T) {
	e := newStatusLineTestEngine("repo: {{.repo}}")

	got, err := e.StatusLineText()
	if err != nil {
		t.Fatalf("StatusLineText() unexpected error: %v", err)
	}

	want := "repo: " + e.geom.RepoName
	if got != want {
		t.Errorf("StatusLineText() = %q; want %q", got, want)
	}
}

// TestStatusLineText_ConfiguredTemplateRendersAllThreeTokens pins that the rendered default
// template carries all three token values: a Geometry with distinct RepoName, WorktreeName and
// HubPath must render a string containing each of the three.
func TestStatusLineText_ConfiguredTemplateRendersAllThreeTokens(t *testing.T) {
	geom := Geometry{RepoName: "distinct-repo", WorktreeName: "distinct-worktree", HubPath: "distinct-hub"}
	cfg := Config{}
	e := New(cfg, geom)

	got, err := e.StatusLineText()
	if err != nil {
		t.Fatalf("StatusLineText() unexpected error: %v", err)
	}

	for _, want := range []string{geom.RepoName, geom.WorktreeName, geom.HubPath} {
		if !strings.Contains(got, want) {
			t.Errorf("StatusLineText() = %q; want it to contain %q", got, want)
		}
	}
}

func TestValidateStatusLine_UnknownTopLevelTokenErrors(t *testing.T) {
	e := newStatusLineTestEngine("{{.slug}}")

	if err := e.ValidateStatusLine(); err == nil {
		t.Error("ValidateStatusLine() = nil; want an error for an unknown top-level token")
	}
}

func TestValidateStatusLine_GoodTemplateReturnsNil(t *testing.T) {
	e := newStatusLineTestEngine("repo: {{.repo}}")

	if err := e.ValidateStatusLine(); err != nil {
		t.Errorf("ValidateStatusLine() = %v; want nil", err)
	}
}
