package shedadapters

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// evidenceEntry is one labelled ledger entry of a circling-evidence fixture.
type evidenceEntry struct {
	key, status, class, severity string
}

// writeEvidenceLedger writes round's ledger with the given labelled entries.
func writeEvidenceLedger(t *testing.T, dir string, round int, entries ...evidenceEntry) {
	t.Helper()
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  - key: %s\n    status: %s\n    rounds: [%d]\n", e.key, e.status, round)
		if e.class != "" {
			fmt.Fprintf(&b, "    class: %s\n", e.class)
		}
		if e.severity != "" {
			fmt.Fprintf(&b, "    severity: %s\n", e.severity)
		}
	}
	list := "ledger: []\n"
	if len(entries) > 0 {
		list = "ledger:\n" + b.String()
	}
	content := fmt.Sprintf("---\nround: %d\n%s---\nprose\n", round, list)
	if err := os.WriteFile(ledgerPath(dir, round), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile ledger round %d = %v; want nil", round, err)
	}
}

func TestCirclingEvidence(t *testing.T) {
	gating := func(key, status string) evidenceEntry {
		return evidenceEntry{key, status, "design", "MEDIUM"}
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  string
	}{
		{
			name: "gating key open in rounds 1 and 3, resolved in 2, is evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha", "open"))
				writeEvidenceLedger(t, dir, 2, gating("alpha", "resolved"))
				writeEvidenceLedger(t, dir, 3, gating("alpha", "open"))
			},
			want: "[alpha]",
		},
		{
			name: "unchanged gating key open in 2 and 3 is evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 2, gating("alpha", "open"))
				writeEvidenceLedger(t, dir, 3, gating("alpha", "open"))
			},
			want: "[alpha]",
		},
		{
			name: "blocking scope key open in 2 and 3 is evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 2, evidenceEntry{"beta", "open", "scope", "BLOCKING"})
				writeEvidenceLedger(t, dir, 3, evidenceEntry{"beta", "open", "scope", "BLOCKING"})
			},
			want: "[beta]",
		},
		{
			name: "non-gating recurring key is not evidence",
			setup: func(t *testing.T, dir string) {
				low := evidenceEntry{"alpha", "open", "design", "LOW"}
				writeEvidenceLedger(t, dir, 2, low)
				writeEvidenceLedger(t, dir, 3, low)
			},
			want: "[]",
		},
		{
			name: "unlabelled recurring key is not evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 2, evidenceEntry{key: "alpha", status: "open"})
				writeEvidenceLedger(t, dir, 3, evidenceEntry{key: "alpha", status: "open"})
			},
			want: "[]",
		},
		{
			name: "new gating keys only at round 3 are not evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("old", "open"))
				writeEvidenceLedger(t, dir, 2, gating("old", "resolved"))
				writeEvidenceLedger(t, dir, 3, gating("n1", "open"), gating("n2", "open"), gating("n3", "open"))
			},
			want: "[]",
		},
		{
			name: "key resolved in ledger 3 is not evidence",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 2, gating("alpha", "open"))
				writeEvidenceLedger(t, dir, 3, gating("alpha", "resolved"))
			},
			want: "[]",
		},
		{
			name: "missing ledger 3 gives none",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha", "open"))
				writeEvidenceLedger(t, dir, 2, gating("alpha", "open"))
			},
			want: "[]",
		},
		{
			name: "unparseable earlier ledger contributes no rounds",
			setup: func(t *testing.T, dir string) {
				if err := os.WriteFile(ledgerPath(dir, 2), []byte("not a ledger"), 0o644); err != nil {
					t.Fatalf("WriteFile = %v; want nil", err)
				}
				writeEvidenceLedger(t, dir, 3, gating("alpha", "open"))
			},
			want: "[]",
		},
		{
			name: "evidence keys come back sorted",
			setup: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 2, gating("zeta", "open"), gating("alpha", "open"))
				writeEvidenceLedger(t, dir, 3, gating("zeta", "open"), gating("alpha", "open"))
			},
			want: "[alpha zeta]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			got := fmt.Sprint(circlingEvidence(dir, 3))
			if got != tt.want {
				t.Errorf("circlingEvidence(dir, 3) = %s; want %s", got, tt.want)
			}
		})
	}
}
