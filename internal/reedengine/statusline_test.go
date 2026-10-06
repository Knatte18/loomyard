// statusline_test.go covers StatusLineText and ValidateStatusLine hermetically: an Engine built from
// Config/Geometry struct literals (no lyxcwd.Resolve, no tmux spawn), per the Test Tier
// Purity Invariant.

package reedengine

import (
	"strings"
	"testing"
)

// newStatusLineTestEngine builds a test Engine with the given status-line template and geometry.
func newStatusLineTestEngine(template string, geom Geometry) *Engine {
	return New(Config{StatusLine: StatusLineConfig{Template: template}}, geom)
}

// TestStatusLineText pins the rendered status text: the embedded default template renders all three token values (a Geometry with distinct RepoName, WorktreeName and HubPath must carry each),
// and a configured template renders from the config.
//
//testtiming:keep pins the embedded default template rendering the repo, worktree and hub values and a configured template rendering from the config; its covering tests run this code without asserting it
func TestStatusLineText(t *testing.T) {
	geom := Geometry{RepoName: "distinct-repo", WorktreeName: "distinct-worktree", HubPath: "distinct-hub"}
	tests := []struct {
		name     string
		template string
		want     string
		trim     bool // the embedded default template ends with a newline
	}{
		{"EmptyTemplateRendersEmbeddedDefault", "", "distinct-repo/distinct-worktree · distinct-hub", true},
		{"ConfiguredTemplateRendersFromConfig", "repo: {{.repo}}", "repo: distinct-repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newStatusLineTestEngine(tt.template, geom).StatusLineText()
			if err != nil {
				t.Fatalf("StatusLineText() unexpected error: %v", err)
			}
			if tt.trim {
				got = strings.TrimSpace(got)
			}
			if got != tt.want {
				t.Errorf("StatusLineText() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestValidateStatusLine pins that a template with an unknown top-level token is an error and a good template validates.
//
//testtiming:keep pins a template with an unknown top-level token being an error and a good template validating; its covering tests run this code without asserting it
func TestValidateStatusLine(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{"UnknownTopLevelTokenErrors", "{{.slug}}", true},
		{"GoodTemplateReturnsNil", "repo: {{.repo}}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newStatusLineTestEngine(tt.template, Geometry{RepoName: "test-repo"}).ValidateStatusLine()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStatusLine() = %v; want error: %v", err, tt.wantErr)
			}
		})
	}
}
