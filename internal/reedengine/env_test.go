// env_test.go verifies CleanClaudeEnv strips exactly the CLAUDECODE / CLAUDE_CODE_* keys, leaves
// unrelated keys untouched, and reports the stripped keys in environ order.

package reedengine

import (
	"slices"
	"testing"
)

//testtiming:keep pins CleanClaudeEnv stripping exactly CLAUDECODE and the CLAUDE_CODE_ keys, reporting them in environ order and leaving other keys and an unaffected environ as they were; its covering tests run this code without asserting it
func TestCleanClaudeEnv(t *testing.T) {
	tests := []struct {
		name         string
		environ      []string
		wantClean    []string
		wantStripped []string
	}{
		{
			name: "StripsClaudeKeysInEnvironOrder",
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
			environ:   []string{"HOME=/home/user", "PATH=/usr/bin"},
			wantClean: []string{"HOME=/home/user", "PATH=/usr/bin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, stripped := CleanClaudeEnv(tt.environ)

			if !slices.Equal(clean, tt.wantClean) {
				t.Errorf("clean = %v, want %v", clean, tt.wantClean)
			}
			if !slices.Equal(stripped, tt.wantStripped) {
				t.Errorf("stripped = %v, want %v", stripped, tt.wantStripped)
			}
		})
	}
}
