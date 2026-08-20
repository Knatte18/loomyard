package shedadapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestSplitFrontmatterCommonRules exercises the frontmatter-delimiter and
// unknown-extra-key rules shared by all three file contracts, driven through
// each of the three top-level parsers so a regression in the shared
// splitFrontmatter/frontmatterProse helpers is caught no matter which parser
// exposed it.
func TestParseVerdictCommonRules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing opening delimiter",
			content: "verdict: APPROVED\nrationale: fine\n---\n",
			wantErr: "must open with a \"---\" frontmatter delimiter line",
		},
		{
			name:    "missing closing delimiter",
			content: "---\nverdict: APPROVED\nrationale: fine\n",
			wantErr: "missing its closing \"---\" delimiter line",
		},
		{
			name:    "empty frontmatter",
			content: "---\n\n---\nprose\n",
			wantErr: "frontmatter is empty",
		},
		{
			name:    "invalid YAML",
			content: "---\nverdict: [unterminated\n---\n",
			wantErr: "frontmatter is not valid YAML",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseVerdict([]byte(tt.content))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "bouncer: verdict file") {
				t.Fatalf("error %q is missing the bouncer verdict-kind prefix", err.Error())
			}
		})
	}
}

func TestParseVerdictProseAndUnknownKey(t *testing.T) {
	content := "---\nverdict: APPROVED\nrationale: fine\nunknown_key: noise\n---\r\nline one\r\nline two\r\n"
	verdict, rationale, err := parseVerdict([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict != verdictApproved {
		t.Fatalf("verdict = %q, want %q", verdict, verdictApproved)
	}
	if rationale != "fine" {
		t.Fatalf("rationale = %q, want %q", rationale, "fine")
	}
	prose := frontmatterProse([]byte(content))
	if prose != "line one\nline two" {
		t.Fatalf("prose = %q, want CRLF-normalised %q", prose, "line one\nline two")
	}
}

func TestParseVerdictSpecific(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantOK  bool
		wantV   bouncerVerdict
		wantErr string
	}{
		{
			name:    "APPROVED accepted",
			content: "---\nverdict: APPROVED\nrationale: looks good\n---\n",
			wantOK:  true,
			wantV:   verdictApproved,
		},
		{
			name:    "BLOCKING accepted",
			content: "---\nverdict: BLOCKING\nrationale: needs work\n---\n",
			wantOK:  true,
			wantV:   verdictBlocking,
		},
		{
			name:    "lowercase approved rejected",
			content: "---\nverdict: approved\nrationale: looks good\n---\n",
			wantErr: "verdict must be exactly",
		},
		{
			name:    "unknown verdict rejected",
			content: "---\nverdict: MAYBE\nrationale: looks good\n---\n",
			wantErr: "verdict must be exactly",
		},
		{
			name:    "empty rationale rejected",
			content: "---\nverdict: APPROVED\nrationale: \"\"\n---\n",
			wantErr: "missing a non-empty rationale",
		},
		{
			name:    "whitespace-only rationale rejected",
			content: "---\nverdict: APPROVED\nrationale: \"   \"\n---\n",
			wantErr: "missing a non-empty rationale",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, err := parseVerdict([]byte(tt.content))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if v != tt.wantV {
					t.Fatalf("verdict = %q, want %q", v, tt.wantV)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseLedgerCommonRules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing opening delimiter",
			content: "round: 1\nledger: []\n---\n",
			wantErr: "must open with a \"---\" frontmatter delimiter line",
		},
		{
			name:    "missing closing delimiter",
			content: "---\nround: 1\nledger: []\n",
			wantErr: "missing its closing \"---\" delimiter line",
		},
		{
			name:    "empty frontmatter",
			content: "---\n\n---\nprose\n",
			wantErr: "frontmatter is empty",
		},
		{
			name:    "invalid YAML",
			content: "---\nround: [unterminated\n---\n",
			wantErr: "frontmatter is not valid YAML",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseLedger([]byte(tt.content))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "bouncer: ledger file") {
				t.Fatalf("error %q is missing the bouncer ledger-kind prefix", err.Error())
			}
		})
	}
}

func TestParseLedgerProseAndUnknownKey(t *testing.T) {
	content := "---\nround: 2\nledger: []\nunknown_key: noise\n---\r\nnarrative one\r\nnarrative two\r\n"
	lf, err := parseLedger([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lf.Round != 2 {
		t.Fatalf("round = %d, want 2", lf.Round)
	}
	if lf.Prose != "narrative one\nnarrative two" {
		t.Fatalf("prose = %q, want CRLF-normalised", lf.Prose)
	}
}

func TestParseLedgerSpecific(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantOK  bool
		wantErr string
	}{
		{
			name:    "empty ledger list legal",
			content: "---\nround: 1\nledger: []\n---\n",
			wantOK:  true,
		},
		{
			name:    "empty key rejected",
			content: "---\nround: 1\nledger:\n  - key: \"\"\n    rounds: [1]\n    status: open\n---\n",
			wantErr: "empty key",
		},
		{
			name:    "empty rounds list rejected",
			content: "---\nround: 1\nledger:\n  - key: finding-1\n    rounds: []\n    status: open\n---\n",
			wantErr: "empty rounds list",
		},
		{
			name:    "zero round member rejected",
			content: "---\nround: 1\nledger:\n  - key: finding-1\n    rounds: [0]\n    status: open\n---\n",
			wantErr: "non-positive round number",
		},
		{
			name:    "negative round member rejected",
			content: "---\nround: 1\nledger:\n  - key: finding-1\n    rounds: [-1]\n    status: open\n---\n",
			wantErr: "non-positive round number",
		},
		{
			name:    "status outside vocabulary rejected",
			content: "---\nround: 1\nledger:\n  - key: finding-1\n    rounds: [1]\n    status: pending\n---\n",
			wantErr: "want exactly \"open\" or \"resolved\"",
		},
		{
			name:    "zero round rejected",
			content: "---\nround: 0\nledger: []\n---\n",
			wantErr: "round must be a positive integer",
		},
		{
			name:    "negative round rejected",
			content: "---\nround: -1\nledger: []\n---\n",
			wantErr: "round must be a positive integer",
		},
		{
			name:    "dropped carry-forward key parses cleanly",
			content: "---\nround: 2\nledger:\n  - key: finding-2\n    rounds: [2]\n    status: open\n---\n",
			wantOK:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lf, err := parseLedger([]byte(tt.content))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if lf.Entries == nil {
					t.Fatalf("Entries must be non-nil even when empty")
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseFocusCommonRules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing opening delimiter",
			content: "round: 1\nexclude_lenses: []\nfocus: []\n---\n",
			wantErr: "must open with a \"---\" frontmatter delimiter line",
		},
		{
			name:    "missing closing delimiter",
			content: "---\nround: 1\nexclude_lenses: []\nfocus: []\n",
			wantErr: "missing its closing \"---\" delimiter line",
		},
		{
			name:    "empty frontmatter",
			content: "---\n\n---\nprose\n",
			wantErr: "frontmatter is empty",
		},
		{
			name:    "invalid YAML",
			content: "---\nround: [unterminated\n---\n",
			wantErr: "frontmatter is not valid YAML",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFocus([]byte(tt.content))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "bouncer: focus file") {
				t.Fatalf("error %q is missing the bouncer focus-kind prefix", err.Error())
			}
		})
	}
}

func TestParseFocusProseAndUnknownKey(t *testing.T) {
	content := "---\nround: 3\nexclude_lenses: []\nfocus: []\nunknown_key: noise\n---\r\nprose one\r\nprose two\r\n"
	ff, err := parseFocus([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ff.Round != 3 {
		t.Fatalf("round = %d, want 3", ff.Round)
	}
	if ff.Prose != "prose one\nprose two" {
		t.Fatalf("prose = %q, want CRLF-normalised", ff.Prose)
	}
}

func TestParseFocusSpecific(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantOK  bool
		wantErr string
	}{
		{
			name:    "empty exclude_lenses and focus both legal",
			content: "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n",
			wantOK:  true,
		},
		{
			name:    "zero round rejected",
			content: "---\nround: 0\nexclude_lenses: []\nfocus: []\n---\n",
			wantErr: "round must be a positive integer",
		},
		{
			name:    "negative round rejected",
			content: "---\nround: -1\nexclude_lenses: []\nfocus: []\n---\n",
			wantErr: "round must be a positive integer",
		},
		{
			name:    "scalar in place of focus list rejected",
			content: "---\nround: 1\nexclude_lenses: []\nfocus: not-a-list\n---\n",
			wantErr: "frontmatter is not valid YAML",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff, err := parseFocus([]byte(tt.content))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ff.ExcludeLenses == nil || ff.Focus == nil {
					t.Fatalf("ExcludeLenses and Focus must both be non-nil even when empty")
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestRenderFocus_RoundTripsThroughParseFocus asserts that
// parseFocus(renderFocus(f)) yields back a value equal to f, for a range of
// focusFile shapes including the seed-fallback shape, a lens name carrying
// YAML metacharacters, and a value carrying prose.
func TestRenderFocus_RoundTripsThroughParseFocus(t *testing.T) {
	tests := []struct {
		name string
		f    focusFile
	}{
		{
			name: "seed-fallback shape: round 1, both lists empty",
			f:    focusFile{Round: 1, ExcludeLenses: []string{}, Focus: []string{}},
		},
		{
			name: "nil lists render as empty lists",
			f:    focusFile{Round: 1},
		},
		{
			name: "populated lists",
			f: focusFile{
				Round:         4,
				ExcludeLenses: []string{"security", "performance"},
				Focus:         []string{"the new parser", "the writer round-trip"},
			},
		},
		{
			name: "lens name with a colon and a space",
			f: focusFile{
				Round:         2,
				ExcludeLenses: []string{"lens: with a colon and a space"},
				Focus:         []string{},
			},
		},
		{
			name: "carries prose",
			f: focusFile{
				Round:         3,
				ExcludeLenses: []string{},
				Focus:         []string{"tighten the error messages"},
				Prose:         "the judge found the error text too terse in round 2.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered, err := renderFocus(tt.f)
			if err != nil {
				t.Fatalf("renderFocus(%+v) = %v; want nil error", tt.f, err)
			}
			got, err := parseFocus(rendered)
			if err != nil {
				t.Fatalf("parseFocus(renderFocus(%+v)) = %v; want nil error", tt.f, err)
			}
			want := tt.f
			if want.ExcludeLenses == nil {
				want.ExcludeLenses = []string{}
			}
			if want.Focus == nil {
				want.Focus = []string{}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("parseFocus(renderFocus(f)) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWriteFocus_WritesReadableFile writes a focusFile into a t.TempDir()
// and reads it back through parseFocus, exercising writeFocus end to end.
func TestWriteFocus_WritesReadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "round-1-focus.md")
	f := focusFile{
		Round:         1,
		ExcludeLenses: []string{},
		Focus:         []string{},
	}

	if err := writeFocus(path, f); err != nil {
		t.Fatalf("writeFocus(%q, %+v) = %v; want nil", path, f, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
	}
	got, err := parseFocus(content)
	if err != nil {
		t.Fatalf("parseFocus(written file) = %v; want nil", err)
	}
	if diff := cmp.Diff(f, got); diff != "" {
		t.Errorf("parseFocus(written file) mismatch (-want +got):\n%s", diff)
	}
}
