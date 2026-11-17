// cli_test.go covers the refusals of `lym retro` that come before any cwd resolution or read.

package retrocli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/retrocli"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRetro_RefusesBadArguments(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"named runs with a since", []string{"a", "--since", "2026-11-01"}, []string{"--since or --until", "not both"}},
		{"named runs with an until", []string{"a", "--until", "2026-11-01"}, []string{"--since or --until", "not both"}},
		{"a since that is no date", []string{"--since", "yesterday"}, []string{"--since", "yesterday", "YYYY-MM-DD"}},
		{"an until with no such month", []string{"--until", "2026-13-01"}, []string{"--until", "2026-13-01", "YYYY-MM-DD"}},
		{"an until later than now", []string{"--until", "2999-01-01"}, []string{"--until", "2999-01-01", "later than now", "omit --until"}},
		{"a cap of zero runs", []string{"--max-runs", "0"}, []string{"--max-runs", "0", "at least 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A temp directory is no git checkout, so only a refusal that precedes cwd resolution can name the flag.
			var out bytes.Buffer
			code := clihelp.ExecuteIn(retrocli.Command(), t.TempDir(), &out, tt.args)

			if code != 1 {
				t.Errorf("exit code = %d; want 1", code)
			}
			for _, text := range tt.want {
				envelope.RequireErr(t, out.String(), text)
			}
		})
	}
}
