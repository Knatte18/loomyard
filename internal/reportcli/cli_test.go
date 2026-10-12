// cli_test.go covers the flag refusals, which come before any cwd resolution or read, and the prime-only refusal decision.

package reportcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestConvergence_RefusesBadFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"a since date with no such month", []string{"--since", "2026-13-01"}, []string{"--since", "2026-13-01", "YYYY-MM-DD"}},
		{"a since that is no date", []string{"--since", "yesterday"}, []string{"--since", "yesterday", "YYYY-MM-DD"}},
		{"a fresh-days of zero", []string{"--fresh-days", "0"}, []string{"--fresh-days", "0", "at least 1"}},
		{"a format that is neither json nor markdown", []string{"--format", "yaml"}, []string{"--format", "yaml", "json", "markdown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A temp directory is no git checkout, so only a refusal that precedes cwd resolution can name the flag.
			var out bytes.Buffer
			code := RunCLIIn(t.TempDir(), &out, append([]string{"convergence"}, tt.args...))

			if code != 1 {
				t.Errorf("exit code = %d; want 1", code)
			}
			for _, text := range tt.want {
				envelope.RequireErr(t, out.String(), text)
			}
		})
	}
}

func TestRefuseNonPrime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		worktree      string
		prime         string
		primeNameErr  error
		wantErr       bool
		wantInMessage []string
	}{
		{name: "the prime's own name passes", worktree: "main", prime: "main"},
		{name: "another name is refused naming the prime", worktree: "task", prime: "main", wantErr: true, wantInMessage: []string{`"task"`, `"main"`, "prime worktree only"}},
		{name: "an unresolvable prime name is refused wrapping its cause", worktree: "main", primeNameErr: errors.New("boom"), wantErr: true, wantInMessage: []string{"boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := refuseNonPrime(tt.worktree, tt.prime, tt.primeNameErr)

			if (err != nil) != tt.wantErr {
				t.Fatalf("refuseNonPrime error = %v; want error %v", err, tt.wantErr)
			}
			for _, text := range tt.wantInMessage {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("error = %q; want it to contain %q", err, text)
				}
			}
			if tt.primeNameErr != nil && !errors.Is(err, tt.primeNameErr) {
				t.Errorf("error = %v; want it to wrap %v", err, tt.primeNameErr)
			}
		})
	}
}
