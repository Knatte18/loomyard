package shedadapters

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/google/go-cmp/cmp"
)

// bouncerFileParsers are the three top-level parsers of the bouncer file contracts, each reduced to
// the error it returns and a summary of the fields its own frontmatter carries plus its prose.
var bouncerFileParsers = map[string]func(content []byte) (summary, prose string, err error){
	"verdict": func(content []byte) (string, string, error) {
		verdict, rationale, err := parseVerdict(content)
		return string(verdict) + "/" + rationale, frontmatterProse(content), err
	},
	"ledger": func(content []byte) (string, string, error) {
		lf, err := parseLedger(content)
		return "round " + strconv.Itoa(lf.Round), lf.Prose, err
	},
	"focus": func(content []byte) (string, string, error) {
		ff, err := parseFocus(content)
		return "round " + strconv.Itoa(ff.Round), ff.Prose, err
	},
}

// TestParseCommonRules exercises the frontmatter-delimiter rules shared by all three file contracts,
// driven through each of the three top-level parsers so a regression in the shared
// splitFrontmatter/frontmatterProse helpers is caught no matter which parser exposed it.
func TestParseCommonRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing opening delimiter",
			content: "round: 1\n---\n",
			wantErr: "must open with a \"---\" frontmatter delimiter line",
		},
		{
			name:    "missing closing delimiter",
			content: "---\nround: 1\n",
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
	for kind, parse := range bouncerFileParsers {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				_, _, err := parse([]byte(tt.content))
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				if !strings.HasPrefix(err.Error(), "bouncer: "+kind+" file") {
					t.Fatalf("error %q is missing the bouncer %s-kind prefix", err.Error(), kind)
				}
			})
		}
	}
}

// TestParseProseAndUnknownKey pins that every contract tolerates an unknown extra frontmatter key
// and returns its prose with CRLF normalised to LF.
//
//testtiming:keep pins that each file contract tolerates an unknown frontmatter key and normalises CRLF prose to LF
func TestParseProseAndUnknownKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind        string
		content     string
		wantSummary string
		wantProse   string
	}{
		{
			kind:        "verdict",
			content:     "---\nverdict: CONVERGED\nrationale: fine\nunknown_key: noise\n---\r\nline one\r\nline two\r\n",
			wantSummary: "CONVERGED/fine",
			wantProse:   "line one\nline two",
		},
		{
			kind:        "ledger",
			content:     "---\nround: 2\nledger: []\nunknown_key: noise\n---\r\nnarrative one\r\nnarrative two\r\n",
			wantSummary: "round 2",
			wantProse:   "narrative one\nnarrative two",
		},
		{
			kind:        "focus",
			content:     "---\nround: 3\nexclude_lenses: []\nfocus: []\nunknown_key: noise\n---\r\nprose one\r\nprose two\r\n",
			wantSummary: "round 3",
			wantProse:   "prose one\nprose two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			summary, prose, err := bouncerFileParsers[tt.kind]([]byte(tt.content))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tt.wantSummary)
			}
			if prose != tt.wantProse {
				t.Errorf("prose = %q, want CRLF-normalised %q", prose, tt.wantProse)
			}
		})
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
			name:    "CONVERGED accepted",
			content: "---\nverdict: CONVERGED\nrationale: looks good\n---\n",
			wantOK:  true,
			wantV:   verdictConverged,
		},
		{
			name:    "CONTINUE accepted",
			content: "---\nverdict: CONTINUE\nrationale: needs work\n---\n",
			wantOK:  true,
			wantV:   verdictContinue,
		},
		{
			name:    "CIRCLING accepted",
			content: "---\nverdict: CIRCLING\nrationale: no progress\n---\n",
			wantOK:  true,
			wantV:   verdictCircling,
		},
		{
			name:    "legacy APPROVED rejected",
			content: "---\nverdict: APPROVED\nrationale: looks good\n---\n",
			wantErr: "verdict must be exactly",
		},
		{
			name:    "lowercase converged rejected",
			content: "---\nverdict: converged\nrationale: looks good\n---\n",
			wantErr: "verdict must be exactly",
		},
		{
			name:    "unknown verdict rejected",
			content: "---\nverdict: MAYBE\nrationale: looks good\n---\n",
			wantErr: "verdict must be exactly",
		},
		{
			name:    "empty rationale rejected",
			content: "---\nverdict: CONVERGED\nrationale: \"\"\n---\n",
			wantErr: "missing a non-empty rationale",
		},
		{
			name:    "whitespace-only rationale rejected",
			content: "---\nverdict: CONVERGED\nrationale: \"   \"\n---\n",
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

func TestParseLedger_ClassAndSeverity(t *testing.T) {
	tests := []struct {
		name         string
		entry        string
		wantClass    burlerengine.Class
		wantSeverity burlerengine.Severity
		wantErr      string
	}{
		{
			name:         "valid class and severity carried",
			entry:        "    class: design\n    severity: MEDIUM\n",
			wantClass:    burlerengine.ClassDesign,
			wantSeverity: burlerengine.SeverityMedium,
		},
		{
			name:  "neither parses with both empty",
			entry: "",
		},
		{
			name:    "class outside vocabulary names the key",
			entry:   "    class: nonsense\n    severity: LOW\n",
			wantErr: `entry "finding-1" has class "nonsense"`,
		},
		{
			name:    "severity outside vocabulary names the key",
			entry:   "    class: scope\n    severity: HUGE\n",
			wantErr: `entry "finding-1" has severity "HUGE"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\nround: 1\nledger:\n  - key: finding-1\n    rounds: [1]\n    status: open\n" + tt.entry + "---\n"
			lf, err := parseLedger([]byte(content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := lf.Entries[0]; got.Class != tt.wantClass || got.Severity != tt.wantSeverity {
				t.Fatalf("entry class/severity = %q/%q, want %q/%q", got.Class, got.Severity, tt.wantClass, tt.wantSeverity)
			}
		})
	}
}

//testtiming:keep pins the focus frontmatter's accept and reject rows (round bounds, scalar focus, absent exclude_lenses), which the bouncer tests reach only through well-formed files
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
			name:    "absent exclude_lenses key legal",
			content: "---\nround: 1\nfocus: []\n---\n",
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
				if len(ff.ExcludeLenses) != 0 || len(ff.Focus) != 0 {
					t.Fatalf("ExcludeLenses and Focus must both be empty, got %v and %v", ff.ExcludeLenses, ff.Focus)
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
//
//testtiming:keep pins the render and parse round trip for shapes the bouncer tests never write: nil lists, YAML metacharacters in a lens name, and prose
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

// writeVerdictAndLedger writes a well-formed verdict file and a ledger file (whose own frontmatter
// round is ledgerRound, independent of the path round its filename encodes) into runDir at round's
// own verdictPath/ledgerPath, for recordedVerdict's own tests.
func writeVerdictAndLedger(t *testing.T, runDir string, round, ledgerRound int) {
	t.Helper()
	verdict := "---\nverdict: CONVERGED\nrationale: \"looks good\"\n---\n"
	if err := os.WriteFile(verdictPath(runDir, round), []byte(verdict), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict): %v", err)
	}
	ledger := "---\nround: " + strconv.Itoa(ledgerRound) + "\nledger: []\n---\n"
	if err := os.WriteFile(ledgerPath(runDir, round), []byte(ledger), 0o644); err != nil {
		t.Fatalf("WriteFile(ledger): %v", err)
	}
}

// TestRecordedVerdict_LedgerRoundMustMatchItsOwnFilename is LS-1's own regression test (crucible
// round sonnet-xhigh-r8): a ledger file whose own round: frontmatter field disagrees with the round
// number its own filename already encodes must not be trusted as "this round has been judged" --
// recordedVerdict's whole job, per its own doc comment.
func TestRecordedVerdict_LedgerRoundMustMatchItsOwnFilename(t *testing.T) {
	t.Run("MatchingRoundIsTrusted", func(t *testing.T) {
		dir := t.TempDir()
		writeVerdictAndLedger(t, dir, 3, 3)

		verdict, judged := recordedVerdict(dir, 3)
		if !judged {
			t.Fatalf("recordedVerdict(round 3) judged = false; want true (a ledger whose own round agrees with its filename)")
		}
		if verdict != verdictConverged {
			t.Errorf("recordedVerdict(round 3) verdict = %q; want %q", verdict, verdictConverged)
		}
	})

	t.Run("MismatchedRoundIsNotTrusted", func(t *testing.T) {
		dir := t.TempDir()
		// The ledger file lands at round-3-*.md (per verdictPath/ledgerPath(dir, 3)) but its own
		// frontmatter claims round: 1 -- the exact shape a judge writing the wrong round's own claim
		// into a file that landed at the right path would produce.
		writeVerdictAndLedger(t, dir, 3, 1)

		_, judged := recordedVerdict(dir, 3)
		if judged {
			t.Error("recordedVerdict(round 3) judged = true; want false -- the ledger's own round: 1 disagrees with its filename's round 3, so it must not be trusted as round 3's judgment")
		}
	})
}
