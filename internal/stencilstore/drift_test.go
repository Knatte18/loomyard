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
			wantClass:    driftSourceAhead,
			wantContains: []string{"update-plugins.sh", "older than the source"},
		},
		{
			name:         "both",
			board:        ApplyStamp([]byte("operator edit\n"), BodyHash([]byte(embedded))),
			source:       ahead,
			wantClass:    driftBoth,
			wantContains: []string{"reconcile by hand", "overwrite the source's changes"},
			wantPromote:  true,
		},
		{
			name:         "neither: untouched copy older than a dev build's embedded bytes",
			board:        stampedBody("older body\n"),
			source:       embedded,
			wantClass:    driftNeither,
			wantContains: []string{"lyx stencil sync", "dev build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			class, msg := classifyPortBackDrift(tt.board, []byte(tt.source), []byte(embedded))
			if class != tt.wantClass {
				t.Errorf("class = %v; want %v", class, tt.wantClass)
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

// TestWarnPortBackDrift_EmitsClassAndRemedyPerDifferingStencil drives the warning end to end:
// a differing stencil warns once naming the stencil and its class, an equal one warns nothing.
// It captures the logger's output, which is process-global state, so it stays serial.
func TestWarnPortBackDrift_EmitsClassAndRemedyPerDifferingStencil(t *testing.T) {
	baseDir := t.TempDir()
	sourceDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{
		"family-older": []byte("embedded body\n"),
		"family-same":  []byte("embedded body\n"),
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

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	warnPortBackDrift(baseDir, registry, sourceDir)

	got := buf.String()
	for _, want := range []string{"family-older", "neither", "lyx stencil sync"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning log = %q; want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "family-same") {
		t.Errorf("warning log = %q; a board copy equal to its source must warn nothing", got)
	}
	if strings.Contains(got, "promote") {
		t.Errorf("warning log = %q; an untouched older copy must never name promote", got)
	}
	if n := strings.Count(got, "board copy has drifted"); n != 1 {
		t.Errorf("warning count = %d; want exactly 1", n)
	}
}
