// judgeverdict_test.go table-drives ParseJudgeVerdict (both framings) and
// ParseTriageVerdict over the happy paths and every fail-loud rule
// documented on them: every legal verdict per framing, the wrong framing's
// vocabulary rejected, lowercase rejected, missing/empty rationale
// rejected, missing/unclosed/empty frontmatter rejected, and CRLF content
// accepted.

package perchengine

import (
	"strings"
	"testing"
)

func TestParseJudgeVerdict(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		framing       judgeFraming
		wantErr       bool
		errSubstr     string
		wantVerdict   JudgeVerdict
		wantRationale string
	}{
		{
			name: "circling framing progressing",
			content: `---
verdict: PROGRESSING
rationale: new findings replaced old ones, no repeats
---

Prose.
`,
			framing:       framingCircling,
			wantVerdict:   JudgeProgressing,
			wantRationale: "new findings replaced old ones, no repeats",
		},
		{
			name: "circling framing circling",
			content: `---
verdict: CIRCLING
rationale: same nil-check finding recurs in rounds 2 and 4
---
`,
			framing:       framingCircling,
			wantVerdict:   JudgeCircling,
			wantRationale: "same nil-check finding recurs in rounds 2 and 4",
		},
		{
			name: "circling framing uncertain",
			content: `---
verdict: UNCERTAIN
rationale: evidence is mixed
---
`,
			framing:       framingCircling,
			wantVerdict:   JudgeUncertain,
			wantRationale: "evidence is mixed",
		},
		{
			name: "milestone framing continue",
			content: `---
verdict: CONTINUE
rationale: findings are shrinking round over round
---
`,
			framing:       framingMilestone,
			wantVerdict:   JudgeContinue,
			wantRationale: "findings are shrinking round over round",
		},
		{
			name: "milestone framing stop",
			content: `---
verdict: STOP
rationale: same two findings oscillate every round
---
`,
			framing:       framingMilestone,
			wantVerdict:   JudgeStop,
			wantRationale: "same two findings oscillate every round",
		},
		{
			name: "milestone framing uncertain",
			content: `---
verdict: UNCERTAIN
rationale: evidence is mixed
---
`,
			framing:       framingMilestone,
			wantVerdict:   JudgeUncertain,
			wantRationale: "evidence is mixed",
		},
		{
			name: "circling framing rejects milestone vocabulary",
			content: `---
verdict: CONTINUE
rationale: not applicable here
---
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "milestone framing rejects circling vocabulary",
			content: `---
verdict: PROGRESSING
rationale: not applicable here
---
`,
			framing:   framingMilestone,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "lowercase rejected",
			content: `---
verdict: progressing
rationale: lowercase should not parse
---
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "missing rationale",
			content: `---
verdict: PROGRESSING
---
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "missing a non-empty rationale",
		},
		{
			name: "empty rationale",
			content: `---
verdict: PROGRESSING
rationale: ""
---
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "missing a non-empty rationale",
		},
		{
			name:      "missing frontmatter",
			content:   "verdict: PROGRESSING\n",
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "must open with a \"---\"",
		},
		{
			name: "unclosed frontmatter",
			content: `---
verdict: PROGRESSING
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "missing its closing",
		},
		{
			name: "empty frontmatter",
			content: `---
---
`,
			framing:   framingCircling,
			wantErr:   true,
			errSubstr: "frontmatter is empty",
		},
		{
			name:          "crlf content",
			content:       "---\r\nverdict: PROGRESSING\r\nrationale: crlf tolerated\r\n---\r\n\r\nProse.\r\n",
			framing:       framingCircling,
			wantVerdict:   JudgeProgressing,
			wantRationale: "crlf tolerated",
		},
		{
			name: "unknown extra header key tolerated",
			content: `---
verdict: PROGRESSING
rationale: extra metadata is harmless
date: 2026-07-08
---
`,
			framing:       framingCircling,
			wantVerdict:   JudgeProgressing,
			wantRationale: "extra metadata is harmless",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, rationale, err := ParseJudgeVerdict([]byte(tt.content), tt.framing)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseJudgeVerdict() = nil error; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseJudgeVerdict() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "perch: ") {
					t.Errorf("ParseJudgeVerdict() error = %q; want perch: -prefixed message", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseJudgeVerdict() = %v; want nil error", err)
			}
			if verdict != tt.wantVerdict {
				t.Errorf("ParseJudgeVerdict() verdict = %q; want %q", verdict, tt.wantVerdict)
			}
			if rationale != tt.wantRationale {
				t.Errorf("ParseJudgeVerdict() rationale = %q; want %q", rationale, tt.wantRationale)
			}
		})
	}
}

func TestParseTriageVerdict(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantErr       bool
		errSubstr     string
		wantVerdict   TriageVerdict
		wantRationale string
	}{
		{
			name: "retry",
			content: `---
verdict: RETRY
rationale: agent asked for confirmation it did not need
---
`,
			wantVerdict:   TriageRetry,
			wantRationale: "agent asked for confirmation it did not need",
		},
		{
			name: "give up",
			content: `---
verdict: GIVE_UP
rationale: the fasit file referenced does not exist
---
`,
			wantVerdict:   TriageGiveUp,
			wantRationale: "the fasit file referenced does not exist",
		},
		{
			name: "lowercase rejected",
			content: `---
verdict: retry
rationale: lowercase should not parse
---
`,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "unknown verdict rejected",
			content: `---
verdict: MAYBE
rationale: unknown spelling
---
`,
			wantErr:   true,
			errSubstr: "verdict must be exactly",
		},
		{
			name: "missing rationale",
			content: `---
verdict: RETRY
---
`,
			wantErr:   true,
			errSubstr: "missing a non-empty rationale",
		},
		{
			name:      "missing frontmatter",
			content:   "verdict: RETRY\n",
			wantErr:   true,
			errSubstr: "must open with a \"---\"",
		},
		{
			name: "unclosed frontmatter",
			content: `---
verdict: RETRY
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
			name:          "crlf content",
			content:       "---\r\nverdict: RETRY\r\nrationale: crlf tolerated\r\n---\r\n",
			wantVerdict:   TriageRetry,
			wantRationale: "crlf tolerated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, rationale, err := ParseTriageVerdict([]byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTriageVerdict() = nil error; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseTriageVerdict() error = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				if !strings.HasPrefix(err.Error(), "perch: ") {
					t.Errorf("ParseTriageVerdict() error = %q; want perch: -prefixed message", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseTriageVerdict() = %v; want nil error", err)
			}
			if verdict != tt.wantVerdict {
				t.Errorf("ParseTriageVerdict() verdict = %q; want %q", verdict, tt.wantVerdict)
			}
			if rationale != tt.wantRationale {
				t.Errorf("ParseTriageVerdict() rationale = %q; want %q", rationale, tt.wantRationale)
			}
		})
	}
}
