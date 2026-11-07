package clihelp_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// readableRoot builds a root wired as cmd/lym wires it, whose output reads as a terminal when terminal is set.
// Its verbs print the envelope {"ok":true,"slug":"x"}.
// `operator` and `role` are verbs of their audience; `own` is an operator verb with a json flag of its own;
// `unterminated` is an operator verb whose envelope has no line break.
func readableRoot(terminal bool) *cobra.Command {
	root := &cobra.Command{
		Use: "lym",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			clihelp.UseReadableOutput(cmd, func(io.Writer) bool { return terminal })
			return nil
		},
	}
	clihelp.InstallJSONHelp(root)
	clihelp.InstallReadableOutput(root)
	verb := func(name, audience string) *cobra.Command {
		return &cobra.Command{
			Use:         name,
			Annotations: map[string]string{clihelp.AudienceAnnotation: audience},
			RunE: func(cmd *cobra.Command, args []string) error {
				output.Ok(cmd.OutOrStdout(), map[string]any{"slug": "x"})
				return nil
			},
		}
	}
	own := verb("own", clihelp.AudienceOperator)
	own.Flags().Bool("json", false, "")
	unterminated := &cobra.Command{
		Use:         "unterminated",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), `{"ok":true}`)
			return nil
		},
	}
	localEnvelope := verb("localenvelope", clihelp.AudienceOperator)
	localEnvelope.Flags().Bool(output.EnvelopeFlag, false, "")
	failing := &cobra.Command{
		Use:         "failing",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), `{"ok":true}`)
			return errors.New("boom")
		},
	}
	root.AddCommand(verb("operator", clihelp.AudienceOperator), verb("role", clihelp.AudienceRole), own, unterminated, localEnvelope, failing)
	return root
}

// TestUseReadableOutput pins which invocations print the readable summary and which keep the envelope.
func TestUseReadableOutput(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const envelope = "{\"ok\":true,\"slug\":\"x\"}\n"
	tests := []struct {
		name     string
		terminal bool
		args     []string
		want     string
		wantCode int
	}{
		{"OperatorOnTerminal_Summary", true, []string{"operator"}, "ok\nslug: x\n", 0},
		{"EnvelopeFlag_Envelope", true, []string{"operator", "--envelope"}, envelope, 0},
		{"RoleAudience_Envelope", true, []string{"role"}, envelope, 0},
		{"NoTerminal_Envelope", false, []string{"operator"}, envelope, 0},
		{"OwnJSONFlag_Envelope", true, []string{"own"}, envelope, 0},
		{"OwnEnvelopeFlag_Envelope", true, []string{"localenvelope", "--envelope"}, envelope, 0},
		{"UnterminatedEnvelope_SummaryAtEnd", true, []string{"unterminated"}, "ok\n", 0},
		{"UnterminatedEnvelopeThenError_SummaryBeforeErrorEnvelope", true, []string{"failing"}, "ok\n{\"error\":\"boom\",\"ok\":false}\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := clihelp.Execute(readableRoot(tt.terminal), &out, tt.args); code != tt.wantCode {
				t.Fatalf("Execute(%v) = %d; want %d, output %q", tt.args, code, tt.wantCode, out.String())
			}
			if got := out.String(); got != tt.want {
				t.Errorf("Execute(%v) output = %q; want %q", tt.args, got, tt.want)
			}
		})
	}
}
