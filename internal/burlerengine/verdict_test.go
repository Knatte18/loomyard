// verdict_test.go table-drives ParseReview over the happy paths and every
// fail-loud rule documented on it: frontmatter presence/closure, YAML
// validity, verdict spelling, per-finding key completeness, severity
// vocabulary, duplicate ids, and the two verdict/findings consistency rules.

package burlerengine

import (
	"strings"
	"testing"
)

func TestParseReview(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantErr      bool
		errSubstr    string
		wantVerdict  Verdict
		wantFindings []Finding
	}{
		{
			name: "happy approved no findings",
			content: `---
verdict: APPROVED
---

Nothing to report.
`,
			wantVerdict:  VerdictApproved,
			wantFindings: nil,
		},
		{
			name: "happy approved nit only findings",
			content: `---
verdict: APPROVED
findings:
  - id: nit-1
    severity: NIT
    location: file.go:10
    summary: prefer a shorter variable name
---

Polish only.
`,
			wantVerdict: VerdictApproved,
			wantFindings: []Finding{
				{ID: "nit-1", Severity: SeverityNit, Location: "file.go:10", Summary: "prefer a shorter variable name"},
			},
		},
		{
			name: "happy blocking mixed severities",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    location: file.go:5
    summary: missing nil check
  - id: m-1
    severity: MEDIUM
    location: file.go:20
    summary: unclear naming
---

Fix the nil check.
`,
			wantVerdict: VerdictBlocking,
			wantFindings: []Finding{
				{ID: "b-1", Severity: SeverityBlocking, Location: "file.go:5", Summary: "missing nil check"},
				{ID: "m-1", Severity: SeverityMedium, Location: "file.go:20", Summary: "unclear naming"},
			},
		},
		{
			name:      "missing frontmatter",
			content:   "verdict: APPROVED\n",
			wantErr:   true,
			errSubstr: "must open with a \"---\"",
		},
		{
			name: "unclosed frontmatter",
			content: `---
verdict: APPROVED
`,
			wantErr:   true,
			errSubstr: "missing its closing",
		},
		{
			name: "bad yaml",
			content: `---
verdict: [APPROVED
---
`,
			wantErr:   true,
			errSubstr: "not valid YAML",
		},
		{
			name: "unknown verdict",
			content: `---
verdict: MAYBE
---
`,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "lowercase approved",
			content: `---
verdict: approved
---
`,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "missing finding id",
			content: `---
verdict: BLOCKING
findings:
  - severity: BLOCKING
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "missing a non-empty id",
		},
		{
			name: "missing finding severity",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "missing a non-empty severity",
		},
		{
			name: "missing finding location",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "missing a non-empty location",
		},
		{
			name: "missing finding summary",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    location: file.go:5
---
`,
			wantErr:   true,
			errSubstr: "missing a non-empty summary",
		},
		{
			name: "unknown severity",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: CRITICAL
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "unknown severity",
		},
		{
			name: "duplicate ids",
			content: `---
verdict: BLOCKING
findings:
  - id: dup
    severity: BLOCKING
    location: file.go:5
    summary: missing nil check
  - id: dup
    severity: LOW
    location: file.go:9
    summary: also here
---
`,
			wantErr:   true,
			errSubstr: "duplicate finding id",
		},
		{
			name: "blocking without blocking finding",
			content: `---
verdict: BLOCKING
findings:
  - id: m-1
    severity: MEDIUM
    location: file.go:5
    summary: not blocking
---
`,
			wantErr:   true,
			errSubstr: "zero BLOCKING-severity findings",
		},
		{
			name: "approved with blocking finding",
			content: `---
verdict: APPROVED
findings:
  - id: b-1
    severity: BLOCKING
    location: file.go:5
    summary: this should not be approved
---
`,
			wantErr:   true,
			errSubstr: "self-contradictory review file",
		},
		{
			name: "unknown extra header key tolerated",
			content: `---
verdict: APPROVED
date: 2026-07-08
reviewer: agent-42
---

Extra metadata is harmless.
`,
			wantVerdict:  VerdictApproved,
			wantFindings: nil,
		},
		{
			name:        "crlf content",
			content:     "---\r\nverdict: APPROVED\r\n---\r\n\r\nAll clear.\r\n",
			wantVerdict: VerdictApproved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, findings, err := ParseReview([]byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseReview() = nil error; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseReview() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "burler: ") {
					t.Errorf("ParseReview() error = %q; want burler: -prefixed message", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseReview() = %v; want nil error", err)
			}
			if verdict != tt.wantVerdict {
				t.Errorf("ParseReview() verdict = %q; want %q", verdict, tt.wantVerdict)
			}
			if len(findings) != len(tt.wantFindings) {
				t.Fatalf("ParseReview() findings = %+v; want %+v", findings, tt.wantFindings)
			}
			for i := range findings {
				if findings[i] != tt.wantFindings[i] {
					t.Errorf("ParseReview() findings[%d] = %+v; want %+v", i, findings[i], tt.wantFindings[i])
				}
			}
		})
	}
}
