// envscrub_test.go verifies EnvScrub strips exactly the keys it is told and reports them in environ order.

package reedengine

import (
	"slices"
	"testing"
)

// testScrub is Claude's declared scrub, spelled out because this package's tests cannot import the registry that owns it.
var testScrub = EnvScrub{Names: []string{"CLAUDECODE"}, Prefixes: []string{"CLAUDE_CODE_"}}

//testtiming:keep pins EnvScrub.Apply stripping exactly the told names and prefixes, reporting them in environ order and leaving other keys and an unaffected environ as they were; its covering tests run this code without asserting it
func TestEnvScrub_Apply(t *testing.T) {
	tests := []struct {
		name         string
		scrub        EnvScrub
		environ      []string
		wantClean    []string
		wantStripped []string
	}{
		{
			name:  "StripsClaudeKeysInEnvironOrder",
			scrub: testScrub,
			environ: []string{
				"CLAUDECODE=1",
				"CLAUDE_CODE_SESSION_ID=abc",
				"CLAUDE_CODE_CHILD_SESSION=1",
				"CLAUDE_CODE_ENTRYPOINT=x",
				"CLAUDE_CODE_SSE_PORT=9",
				"HOME=/home/user",
				"PATH=/usr/bin",
				"MY_VAR=ok",
			},
			wantClean: []string{"HOME=/home/user", "PATH=/usr/bin", "MY_VAR=ok"},
			wantStripped: []string{
				"CLAUDECODE",
				"CLAUDE_CODE_SESSION_ID",
				"CLAUDE_CODE_CHILD_SESSION",
				"CLAUDE_CODE_ENTRYPOINT",
				"CLAUDE_CODE_SSE_PORT",
			},
		},
		{
			name:      "NoClaudeKeysUnchanged",
			scrub:     testScrub,
			environ:   []string{"HOME=/home/user", "PATH=/usr/bin"},
			wantClean: []string{"HOME=/home/user", "PATH=/usr/bin"},
		},
		{
			name:         "ScrubsExactlyTheToldKeys",
			scrub:        EnvScrub{Names: []string{"OTHERCODE"}, Prefixes: []string{"OTHER_CODE_"}},
			environ:      []string{"OTHERCODE=1", "OTHER_CODE_X=2", "CLAUDECODE=1", "CLAUDE_CODE_X=2", "HOME=/h"},
			wantClean:    []string{"CLAUDECODE=1", "CLAUDE_CODE_X=2", "HOME=/h"},
			wantStripped: []string{"OTHERCODE", "OTHER_CODE_X"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, stripped := tt.scrub.Apply(tt.environ)

			if !slices.Equal(clean, tt.wantClean) {
				t.Errorf("clean = %v, want %v", clean, tt.wantClean)
			}
			if !slices.Equal(stripped, tt.wantStripped) {
				t.Errorf("stripped = %v, want %v", stripped, tt.wantStripped)
			}
		})
	}
}
