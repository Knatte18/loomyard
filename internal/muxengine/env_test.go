// env_test.go verifies CleanClaudeEnv strips exactly the CLAUDECODE /
// CLAUDE_CODE_* keys, leaves unrelated keys untouched, and reports the
// stripped keys in environ order.

package muxengine

import "testing"

func TestCleanClaudeEnv(t *testing.T) {
	environ := []string{
		"CLAUDECODE=1",
		"CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"CLAUDE_CODE_ENTRYPOINT=x",
		"CLAUDE_CODE_SSE_PORT=9",
		"HOME=/home/user",
		"PATH=/usr/bin",
		"MY_VAR=ok",
	}

	clean, stripped := CleanClaudeEnv(environ)

	wantClean := []string{"HOME=/home/user", "PATH=/usr/bin", "MY_VAR=ok"}
	if len(clean) != len(wantClean) {
		t.Fatalf("clean = %v, want %v", clean, wantClean)
	}
	for i := range wantClean {
		if clean[i] != wantClean[i] {
			t.Errorf("clean[%d] = %q, want %q", i, clean[i], wantClean[i])
		}
	}

	wantStripped := []string{
		"CLAUDECODE",
		"CLAUDE_CODE_SESSION_ID",
		"CLAUDE_CODE_CHILD_SESSION",
		"CLAUDE_CODE_ENTRYPOINT",
		"CLAUDE_CODE_SSE_PORT",
	}
	if len(stripped) != len(wantStripped) {
		t.Fatalf("stripped = %v, want %v", stripped, wantStripped)
	}
	for i := range wantStripped {
		if stripped[i] != wantStripped[i] {
			t.Errorf("stripped[%d] = %q, want %q", i, stripped[i], wantStripped[i])
		}
	}
}

func TestCleanClaudeEnv_NoClaudeKeysUnchanged(t *testing.T) {
	environ := []string{"HOME=/home/user", "PATH=/usr/bin"}

	clean, stripped := CleanClaudeEnv(environ)

	if len(stripped) != 0 {
		t.Errorf("stripped = %v, want empty", stripped)
	}
	if len(clean) != len(environ) {
		t.Fatalf("clean = %v, want %v", clean, environ)
	}
	for i := range environ {
		if clean[i] != environ[i] {
			t.Errorf("clean[%d] = %q, want %q", i, clean[i], environ[i])
		}
	}
}
