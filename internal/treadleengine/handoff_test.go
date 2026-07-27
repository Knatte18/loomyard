// handoff_test.go table-drives ParseHandoff over the happy path (including
// prose extraction and an empty ledger) and every fail-loud rule documented
// on it: missing/unclosed/empty frontmatter, invalid YAML, empty
// covers_rounds, non-positive round numbers (in covers_rounds and in a
// ledger entry's rounds), a ledger entry with an empty key, an empty rounds
// list, an out-of-vocabulary status, and CRLF-tolerant content.

package treadleengine

import (
	"strings"
	"testing"
)

func TestParseHandoff(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantErr    bool
		errSubstr  string
		wantCovers []int
		wantLedger []LedgerEntry
		wantProse  string
	}{
		{
			name: "happy path with ledger and prose",
			content: `---
covers_rounds: [1, 2, 3]
ledger:
  - key: nil-check-missing
    rounds: [1, 3]
    status: open
  - key: stale-comment
    rounds: [2]
    status: resolved
---

## Handoff

Round 3 fixed the stale comment; the nil-check finding recurs.
`,
			wantCovers: []int{1, 2, 3},
			wantLedger: []LedgerEntry{
				{Key: "nil-check-missing", Rounds: []int{1, 3}, Status: "open"},
				{Key: "stale-comment", Rounds: []int{2}, Status: "resolved"},
			},
			wantProse: "## Handoff\n\nRound 3 fixed the stale comment; the nil-check finding recurs.",
		},
		{
			name: "happy path empty ledger",
			content: `---
covers_rounds: [1]
ledger: []
---

First handoff, nothing to carry forward yet.
`,
			wantCovers: []int{1},
			wantLedger: []LedgerEntry{},
			wantProse:  "First handoff, nothing to carry forward yet.",
		},
		{
			name: "ledger frontmatter key omitted entirely",
			content: `---
covers_rounds: [1]
---

No ledger key at all.
`,
			wantCovers: []int{1},
			wantLedger: []LedgerEntry{},
			wantProse:  "No ledger key at all.",
		},
		{
			name: "invalid yaml",
			content: `---
covers_rounds: [1
---
`,
			wantErr:   true,
			errSubstr: "not valid YAML",
		},
		{
			name: "empty covers_rounds",
			content: `---
covers_rounds: []
---
`,
			wantErr:   true,
			errSubstr: "covers_rounds must be a non-empty list",
		},
		{
			name: "missing covers_rounds",
			content: `---
ledger: []
---
`,
			wantErr:   true,
			errSubstr: "covers_rounds must be a non-empty list",
		},
		{
			name: "non-positive round number in covers_rounds",
			content: `---
covers_rounds: [1, 0, 2]
---
`,
			wantErr:   true,
			errSubstr: "covers_rounds contains non-positive round number 0",
		},
		{
			name: "negative round number in covers_rounds",
			content: `---
covers_rounds: [-1]
---
`,
			wantErr:   true,
			errSubstr: "covers_rounds contains non-positive round number -1",
		},
		{
			name: "ledger entry empty key",
			content: `---
covers_rounds: [1]
ledger:
  - key: ""
    rounds: [1]
    status: open
---
`,
			wantErr:   true,
			errSubstr: "ledger entry with an empty key",
		},
		{
			name: "ledger entry empty rounds",
			content: `---
covers_rounds: [1]
ledger:
  - key: some-finding
    rounds: []
    status: open
---
`,
			wantErr:   true,
			errSubstr: `ledger entry "some-finding" has an empty rounds list`,
		},
		{
			name: "ledger entry non-positive round",
			content: `---
covers_rounds: [1]
ledger:
  - key: some-finding
    rounds: [0]
    status: open
---
`,
			wantErr:   true,
			errSubstr: `ledger entry "some-finding" contains non-positive round number 0`,
		},
		{
			name: "ledger entry status outside vocabulary",
			content: `---
covers_rounds: [1]
ledger:
  - key: some-finding
    rounds: [1]
    status: maybe
---
`,
			wantErr:   true,
			errSubstr: `ledger entry "some-finding" has status "maybe"`,
		},
		{
			name: "ledger entry status wrong case rejected",
			content: `---
covers_rounds: [1]
ledger:
  - key: some-finding
    rounds: [1]
    status: OPEN
---
`,
			wantErr:   true,
			errSubstr: `ledger entry "some-finding" has status "OPEN"`,
		},
		{
			name:      "missing frontmatter",
			content:   "covers_rounds: [1]\n",
			wantErr:   true,
			errSubstr: "must open with a \"---\"",
		},
		{
			name: "unclosed frontmatter",
			content: `---
covers_rounds: [1]
`,
			wantErr:   true,
			errSubstr: "missing its closing",
		},
		{
			name: "empty frontmatter",
			content: `---
---
`,
			wantErr:   true,
			errSubstr: "frontmatter is empty",
		},
		{
			name:       "crlf content",
			content:    "---\r\ncovers_rounds: [1]\r\nledger: []\r\n---\r\n\r\nCRLF prose.\r\n",
			wantCovers: []int{1},
			wantLedger: []LedgerEntry{},
			wantProse:  "CRLF prose.",
		},
		{
			name: "unknown extra header key tolerated",
			content: `---
covers_rounds: [1]
ledger: []
generated_by: judge-round-3
---

Extra metadata is harmless.
`,
			wantCovers: []int{1},
			wantLedger: []LedgerEntry{},
			wantProse:  "Extra metadata is harmless.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHandoff([]byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseHandoff() = nil error; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseHandoff() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "treadle: ") {
					t.Errorf("ParseHandoff() error = %q; want treadle: -prefixed message", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseHandoff() = %v; want nil error", err)
			}
			if !intSlicesEqual(got.CoversRounds, tt.wantCovers) {
				t.Errorf("ParseHandoff() CoversRounds = %v; want %v", got.CoversRounds, tt.wantCovers)
			}
			if !ledgerEntriesEqual(got.Ledger, tt.wantLedger) {
				t.Errorf("ParseHandoff() Ledger = %+v; want %+v", got.Ledger, tt.wantLedger)
			}
			if got.Prose != tt.wantProse {
				t.Errorf("ParseHandoff() Prose = %q; want %q", got.Prose, tt.wantProse)
			}
		})
	}
}

// ledgerEntriesEqual reports whether a and b contain the same LedgerEntry
// values in the same order.
func ledgerEntriesEqual(a, b []LedgerEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Status != b[i].Status || !intSlicesEqual(a[i].Rounds, b[i].Rounds) {
			return false
		}
	}
	return true
}
