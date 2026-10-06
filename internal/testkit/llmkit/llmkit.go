// Package llmkit locates the LLM binary for a test and skips the test when it is absent.
//
// Only `llm`-tagged test files may import it.
// It is the third kit exempt from the Testkit Invariant's `os/exec` ban, after lyxbin and tmuxkit.
// The exemption is bounded to `exec.LookPath`: it locates a binary and starts nothing.
package llmkit

import (
	"os"
	"os/exec"
	"testing"
)

// Claude returns the path of the `claude` binary for a test.
// The value of the environment variable overrideEnv wins when it is set and non-empty, else the `PATH` lookup answers.
// The test is skipped, naming both the variable and `claude`, when neither yields a path.
func Claude(t *testing.T, overrideEnv string) string {
	t.Helper()
	if p := os.Getenv(overrideEnv); p != "" {
		return p
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		t.Skipf("%s is unset and claude is not on PATH", overrideEnv)
	}
	return p
}
