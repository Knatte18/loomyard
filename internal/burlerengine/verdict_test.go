// verdict_test.go table-drives ParseReview over the happy paths and every fail-loud rule documented on it: frontmatter presence/closure, YAML validity, verdict spelling, per-finding key completeness, severity and class vocabularies, duplicate ids, the two verdict/findings consistency rules, and the optional Origin field's pass-through (with and without it present).

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
			name: "happy blocking with focus departures section",
			content: `---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: file.go:7
    summary: unchecked error
---

### [BLOCKING] unchecked error

**Location:** file.go:7

## Focus departures

- "only look at file.go": also read util.go because the error originates there.
`,
			wantVerdict: VerdictBlocking,
			wantFindings: []Finding{
				{ID: "F1", Severity: SeverityBlocking, Class: ClassDesign, Location: "file.go:7", Summary: "unchecked error"},
			},
		},
		{
			name: "happy approved nit only findings",
			content: `---
verdict: APPROVED
findings:
  - id: nit-1
    severity: NIT
    class: consistency
    location: file.go:10
    summary: prefer a shorter variable name
---

Polish only.
`,
			wantVerdict: VerdictApproved,
			wantFindings: []Finding{
				{ID: "nit-1", Severity: SeverityNit, Class: ClassConsistency, Location: "file.go:10", Summary: "prefer a shorter variable name"},
			},
		},
		{
			name: "happy blocking mixed severities",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: design
    location: file.go:5
    summary: missing nil check
  - id: m-1
    severity: MEDIUM
    class: scope
    location: file.go:20
    summary: unclear naming
---

Fix the nil check.
`,
			wantVerdict: VerdictBlocking,
			wantFindings: []Finding{
				{ID: "b-1", Severity: SeverityBlocking, Class: ClassDesign, Location: "file.go:5", Summary: "missing nil check"},
				{ID: "m-1", Severity: SeverityMedium, Class: ClassScope, Location: "file.go:20", Summary: "unclear naming"},
			},
		},
		{
			name: "every class value parses and is carried",
			content: `---
verdict: APPROVED
findings:
  - id: c-1
    severity: LOW
    class: design
    location: file.go:1
    summary: design finding
  - id: c-2
    severity: LOW
    class: scope
    location: file.go:2
    summary: scope finding
  - id: c-3
    severity: LOW
    class: decision
    location: file.go:3
    summary: decision finding
  - id: c-4
    severity: LOW
    class: consistency
    location: file.go:4
    summary: consistency finding
---
`,
			wantVerdict: VerdictApproved,
			wantFindings: []Finding{
				{ID: "c-1", Severity: SeverityLow, Class: ClassDesign, Location: "file.go:1", Summary: "design finding"},
				{ID: "c-2", Severity: SeverityLow, Class: ClassScope, Location: "file.go:2", Summary: "scope finding"},
				{ID: "c-3", Severity: SeverityLow, Class: ClassDecision, Location: "file.go:3", Summary: "decision finding"},
				{ID: "c-4", Severity: SeverityLow, Class: ClassConsistency, Location: "file.go:4", Summary: "consistency finding"},
			},
		},
		{
			// A cluster round's consolidated review tags each finding with an
			// origin: key (lens:<name> or handler) — ParseReview must parse it
			// through onto Finding.Origin without validating its content.
			name: "cluster findings carry origin",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: design
    location: file.go:5
    summary: missing nil check
    origin: lens:security
  - id: m-1
    severity: MEDIUM
    class: design
    location: file.go:20
    summary: unclear naming
    origin: handler
---

Fix the nil check.
`,
			wantVerdict: VerdictBlocking,
			wantFindings: []Finding{
				{ID: "b-1", Severity: SeverityBlocking, Class: ClassDesign, Location: "file.go:5", Summary: "missing nil check", Origin: "lens:security"},
				{ID: "m-1", Severity: SeverityMedium, Class: ClassDesign, Location: "file.go:20", Summary: "unclear naming", Origin: "handler"},
			},
		},
		{
			// A pre-cluster (origin-less) review file must keep parsing
			// identically — origin is optional and never required.
			name: "findings without origin parse identically",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: design
    location: file.go:5
    summary: missing nil check
---

Fix the nil check.
`,
			wantVerdict: VerdictBlocking,
			wantFindings: []Finding{
				{ID: "b-1", Severity: SeverityBlocking, Class: ClassDesign, Location: "file.go:5", Summary: "missing nil check"},
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
			// The single most frequent live review-file defect: a summary
			// that opens with a quoted fragment then carries unquoted
			// trailing prose on the same line — invalid YAML. ParseReview
			// must append a targeted hint rather than just yaml.v3's raw,
			// hard-to-act-on parser error.
			name: "malformed quoted summary appends a targeted hint",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: LOW
    class: design
    location: file.go:1
    summary: "capital" is misspelled as "captial" (both occurrences on line 1)
---
`,
			wantErr:   true,
			errSubstr: "must be ONE double-quoted string covering the whole value",
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
    class: design
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
    class: design
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
    class: design
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
    class: design
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
    class: design
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "unknown severity",
		},
		{
			name: "missing finding class",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "finding \"b-1\" is missing a non-empty class",
		},
		{
			name: "empty finding class",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: "  "
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "finding \"b-1\" is missing a non-empty class",
		},
		{
			name: "unknown finding class wrong case",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: Design
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "unknown class \"Design\"",
		},
		{
			name: "unknown finding class nit",
			content: `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    class: nit
    location: file.go:5
    summary: missing nil check
---
`,
			wantErr:   true,
			errSubstr: "unknown class \"nit\"",
		},
		{
			name: "duplicate ids",
			content: `---
verdict: BLOCKING
findings:
  - id: dup
    severity: BLOCKING
    class: design
    location: file.go:5
    summary: missing nil check
  - id: dup
    severity: LOW
    class: design
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
    class: design
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
    class: design
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
