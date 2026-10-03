// refusalnote.go records the friction note a `lyx webster` refusal leaves, at the one place every verb's output passes through.
// A verb stays unaware of it: noteRefusals wraps the verb's RunE, tees what the verb prints, and reads the envelope back.
package webstercli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// noteRefusals wraps cmd.RunE so a refusal envelope the verb prints also writes one friction note.
// The verb's exit code and envelope are exactly what the original RunE produced: the output is only tee'd, and a note-write failure or an envelope that does not decode logs a warning and changes nothing.
// An `ok: true` envelope, such as `begin-batch`'s `paused: true`, writes nothing, and so does an empty friction directory.
func (c *websterCLI) noteRefusals(cmd *cobra.Command) *cobra.Command {
	inner := cmd.RunE
	if inner == nil {
		return cmd
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		orig := cmd.OutOrStdout()
		var captured bytes.Buffer
		cmd.SetOut(io.MultiWriter(orig, &captured))
		err := inner(cmd, args)
		cmd.SetOut(orig)

		if c.frictionDir == "" {
			return err
		}
		c.noteEnvelope(cmd, args, captured.Bytes())
		return err
	}
	return cmd
}

// noteEnvelope decodes the last non-empty line of out as a JSON envelope and writes a refusal note when its `ok` is false.
func (c *websterCLI) noteEnvelope(cmd *cobra.Command, args []string, out []byte) {
	line := lastNonEmptyLine(out)
	if line == "" {
		return
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		logger.Warn("webstercli: could not decode the verb output as an envelope; no refusal note written", "verb", cmd.Name(), "error", err)
		return
	}
	if ok, _ := envelope["ok"].(bool); ok {
		return
	}

	message, _ := envelope["error"].(string)
	fields := make(map[string]any, len(envelope))
	for k, v := range envelope {
		if k != "ok" && k != "error" {
			fields[k] = v
		}
	}
	note := websterengine.RefusalNote{
		Verb:    cmd.Name(),
		Args:    renderInvocationArgs(cmd, args),
		Message: message,
		Fields:  fields,
	}
	if err := websterengine.WriteRefusalNote(c.frictionDir, note); err != nil {
		logger.Warn("webstercli: could not write the refusal note", "verb", cmd.Name(), "error", err)
	}
}

// lastNonEmptyLine returns the last line of out that holds anything but whitespace, or "" when there is none.
func lastNonEmptyLine(out []byte) string {
	lines := strings.Split(string(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// renderInvocationArgs renders the positional args, then every flag the invocation set as `--name=value`, space-separated.
func renderInvocationArgs(cmd *cobra.Command, args []string) string {
	parts := append([]string(nil), args...)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		parts = append(parts, "--"+f.Name+"="+f.Value.String())
	})
	return strings.Join(parts, " ")
}
