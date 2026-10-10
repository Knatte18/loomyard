package stencilstore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// stampedBody returns body stamped with its own hash, the shape of an untouched board copy.
func stampedBody(body string) []byte {
	return ApplyStamp([]byte(body), BodyHash([]byte(body)))
}

// TestClassifyPortBackDrift pins each of the four classes' message and remedy.
func TestClassifyPortBackDrift(t *testing.T) {
	t.Parallel()

	const embedded = "embedded body\n"
	const ahead = "source ahead body\n"

	tests := []struct {
		name         string
		board        []byte
		source       string
		wantClass    driftClass
		wantContains []string
		wantPromote  bool
		// ancestry is what the build func reports; nilBuild passes no func at all.
		ancestry   BuildAncestry
		nilBuild   bool
		wantCalls  int
		wantNoText []string
	}{
		{
			name:         "hand-edited only",
			board:        ApplyStamp([]byte("operator edit\n"), BodyHash([]byte(embedded))),
			source:       embedded,
			wantClass:    driftHandEdited,
			wantContains: []string{"lyx stencil promote <name>"},
			wantPromote:  true,
		},
		{
			name:         "source-ahead only",
			board:        stampedBody(embedded),
			source:       ahead,
			ancestry:     BuildInHead,
			wantClass:    driftSourceAhead,
			wantContains: []string{"update-plugins.sh", "older than the source"},
			wantCalls:    1,
		},
		{
			name:         "source-ahead with an unknown ancestry",
			board:        stampedBody(embedded),
			source:       ahead,
			ancestry:     BuildAncestryUnknown,
			wantClass:    driftSourceAhead,
			wantContains: []string{"update-plugins.sh"},
			wantCalls:    1,
		},
		{
			name:         "source-ahead with no build func",
			board:        stampedBody(embedded),
			source:       ahead,
			nilBuild:     true,
			wantClass:    driftSourceAhead,
			wantContains: []string{"update-plugins.sh"},
		},
		{
			name:         "source-ahead in a worktree behind the build",
			board:        stampedBody(embedded),
			source:       ahead,
			ancestry:     BuildNotInHead,
			wantClass:    driftBehind,
			wantContains: []string{"behind the build", "syncing"},
			wantNoText:   []string{"update-plugins.sh"},
			wantCalls:    1,
		},
		{
			name:         "both",
			board:        ApplyStamp([]byte("operator edit\n"), BodyHash([]byte(embedded))),
			source:       ahead,
			ancestry:     BuildNotInHead,
			wantClass:    driftBoth,
			wantContains: []string{"reconcile by hand", "overwrite the source's changes"},
			wantPromote:  true,
		},
		{
			name:         "neither: untouched copy older than a dev build's embedded bytes",
			board:        stampedBody("older body\n"),
			source:       embedded,
			ancestry:     BuildNotInHead,
			wantClass:    driftNeither,
			wantContains: []string{"lyx stencil sync", "dev build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			var build func() BuildAncestry
			if !tt.nilBuild {
				build = func() BuildAncestry {
					calls++
					return tt.ancestry
				}
			}
			class, msg := classifyPortBackDrift(tt.board, []byte(tt.source), []byte(embedded), build)
			if class != tt.wantClass {
				t.Errorf("class = %v; want %v", class, tt.wantClass)
			}
			if calls != tt.wantCalls {
				t.Errorf("build calls = %d; want %d", calls, tt.wantCalls)
			}
			for _, unwanted := range tt.wantNoText {
				if strings.Contains(msg, unwanted) {
					t.Errorf("message = %q; want it not to contain %q", msg, unwanted)
				}
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("message = %q; want it to contain %q", msg, want)
				}
			}
			if got := strings.Contains(msg, "promote"); got != tt.wantPromote {
				t.Errorf("message = %q; mentions promote = %v, want %v", msg, got, tt.wantPromote)
			}
		})
	}
}

// TestWarnPortBackDrift_EmitsOneLineListingTheDriftedStencils drives the warning end to end:
// the differing stencils are listed with their classes on one line, with their remedies once, and an equal one is not listed.
// The line is a Warn unless every differing stencil is in the behind class, when it is an Info.
// It captures the logger's output, which is process-global state, so it stays serial.
func TestWarnPortBackDrift_EmitsOneLineListingTheDriftedStencils(t *testing.T) {
	baseDir := t.TempDir()
	sourceDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{
		"family-older": []byte("embedded body\n"),
		"family-same":  []byte("embedded body\n"),
		"family-ahead": []byte("embedded body\n"),
	})

	writeAt := func(root, name string, content []byte) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(RelPath(name)))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeAt(baseDir, "family-older", stampedBody("older body\n"))
	writeAt(sourceDir, "family-older", []byte("embedded body\n"))
	writeAt(baseDir, "family-same", stampedBody("embedded body\n"))
	writeAt(sourceDir, "family-same", []byte("embedded body\n"))
	writeAt(baseDir, "family-ahead", stampedBody("embedded body\n"))
	writeAt(sourceDir, "family-ahead", []byte("source ahead body\n"))

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})

	buildCalls := 0
	warnPortBackDrift(baseDir, registry, Source{Dir: sourceDir, Build: func() BuildAncestry {
		buildCalls++
		return BuildNotInHead
	}})

	got := buf.String()
	// Only the source-ahead stencil reaches the ancestry read; the equal and older ones never do.
	if buildCalls != 1 {
		t.Errorf("build calls = %d; want 1, from the source-ahead stencil alone", buildCalls)
	}
	if !strings.Contains(got, "lyx stencil sync") {
		t.Errorf("warning log = %q; want it to name the sync remedy of the neither class", got)
	}
	if lines := strings.Split(strings.TrimSpace(got), "\n"); len(lines) != 1 {
		t.Fatalf("warning log = %q; want exactly one line for the whole pass", got)
	}
	if !strings.Contains(got, "level=WARN") || !strings.Contains(got, "2 stencils") {
		t.Errorf("warning log = %q; want a Warn naming the count 2 because a stencil outside the behind class differs", got)
	}
	for _, want := range []string{"family-older: neither", "family-ahead: behind"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning log = %q; want it to list %q", got, want)
		}
	}
	if strings.Contains(got, "family-same") {
		t.Errorf("warning log = %q; a board copy equal to its source must not be listed", got)
	}
	if strings.Contains(got, "promote") {
		t.Errorf("warning log = %q; an untouched older copy must never name promote", got)
	}
	if n := strings.Count(got, "syncing this worktree with main"); n != 1 {
		t.Errorf("remedy count = %d; want the behind remedy named once", n)
	}

	// A pass where only the behind class differs logs its one line at Info.
	buf.Reset()
	warnPortBackDrift(baseDir, newFakeRegistry(map[string][]byte{
		"family-same":  []byte("embedded body\n"),
		"family-ahead": []byte("embedded body\n"),
	}), Source{Dir: sourceDir, Build: func() BuildAncestry { return BuildNotInHead }})
	if only := buf.String(); !strings.Contains(only, "level=INFO") || !strings.Contains(only, "1 stencil ") || strings.Contains(only, "level=WARN") {
		t.Errorf("behind-only log = %q; want one Info line naming the count 1", only)
	}
}
