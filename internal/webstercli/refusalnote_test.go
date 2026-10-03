// refusalnote_test.go covers noteRefusals and addVerbs over a hand-built websterCLI and a temporary friction directory.
// No hub is needed: every refusal here fires before a verb reaches the plan, state or fabric.
package webstercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// runVerbs executes args against a bare parent carrying c's verbs, noted through addVerbs or raw, and returns the exit code and output.
func runVerbs(c *websterCLI, noted bool, args ...string) (int, string) {
	parent := &cobra.Command{Use: "webster", RunE: clihelp.GroupRunE}
	if noted {
		c.addVerbs(parent)
	} else {
		parent.AddCommand(c.validateCmd(), c.beginBatchCmd(), c.recoverBatchCmd(), c.rebaselineCmd())
	}
	var out bytes.Buffer
	code := clihelp.Execute(parent, &out, args)
	return code, out.String()
}

// refusalNotes returns the contents of every refusal note in dir, keyed by file name.
func refusalNotes(t *testing.T, dir string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "webster-refusal-*.md"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	notes := make(map[string]string, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		notes[filepath.Base(p)] = string(data)
	}
	return notes
}

// stubRefusal returns a verb that prints an ErrFields refusal carrying plan_drifted.
func stubRefusal(c *websterCLI) *cobra.Command {
	return c.noteRefusals(&cobra.Command{
		Use:  "stub",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			clihelp.SetExit(cmd.Context(), output.ErrFields(cmd.OutOrStdout(), "webster: the plan drifted", map[string]any{"plan_drifted": true}))
			return nil
		},
	})
}

func TestNoteRefusals_ArgumentRefusals_WriteOneNoteEach(t *testing.T) {
	cases := []struct {
		name string
		args []string
		verb string
		want []string
	}{
		{"begin-batch", []string{"begin-batch", "x"}, "begin-batch", []string{"lyx webster begin-batch refused", "Arguments: x", `"x" is not a valid batch number`}},
		{"recover-batch", []string{"recover-batch", "x"}, "recover-batch", []string{"lyx webster recover-batch refused", "Arguments: x", `"x" is not a valid batch number`}},
		{"rebaseline", []string{"rebaseline", "--card", "abc"}, "rebaseline", []string{"lyx webster rebaseline refused", "Arguments: --card=[abc]", `--card "abc" is not a card number`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			c, _ := newTestCLI(t)
			c.frictionDir = dir

			rawCode, rawOut := runVerbs(c, false, tc.args...)
			code, out := runVerbs(c, true, tc.args...)

			if code != rawCode || out != rawOut {
				t.Errorf("noted run = (%d, %q); want the unwrapped (%d, %q)", code, out, rawCode, rawOut)
			}
			notes := refusalNotes(t, dir)
			if len(notes) != 1 {
				t.Fatalf("notes = %v; want exactly one", notes)
			}
			for name, body := range notes {
				if !strings.HasPrefix(name, "webster-refusal-"+tc.verb) {
					t.Errorf("note name %q; want the prefix webster-refusal-%s", name, tc.verb)
				}
				for _, want := range tc.want {
					if !strings.Contains(body, want) {
						t.Errorf("note missing %q; got:\n%s", want, body)
					}
				}
			}
		})
	}
}

func TestNoteRefusals_ErrFieldsRefusal_NoteCarriesField(t *testing.T) {
	dir := t.TempDir()
	c, _ := newTestCLI(t)
	c.frictionDir = dir
	parent := &cobra.Command{Use: "webster", RunE: clihelp.GroupRunE}
	parent.AddCommand(stubRefusal(c))

	var out bytes.Buffer
	code := clihelp.Execute(parent, &out, []string{"stub"})

	if code != 1 {
		t.Fatalf("exit code = %d; want 1, output: %s", code, out.String())
	}
	notes := refusalNotes(t, dir)
	if len(notes) != 1 {
		t.Fatalf("notes = %v; want exactly one", notes)
	}
	for _, body := range notes {
		for _, want := range []string{"lyx webster stub refused", "webster: the plan drifted", "plan_drifted: true"} {
			if !strings.Contains(body, want) {
				t.Errorf("note missing %q; got:\n%s", want, body)
			}
		}
		if strings.Contains(body, "ok:") || strings.Contains(body, "error:") {
			t.Errorf("note repeats a reserved key; got:\n%s", body)
		}
	}
}

func TestNoteRefusals_OkEnvelope_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	c, _ := newTestCLI(t)
	c.frictionDir = dir
	parent := &cobra.Command{Use: "webster", RunE: clihelp.GroupRunE}
	parent.AddCommand(c.noteRefusals(&cobra.Command{
		Use:  "paused",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			clihelp.SetExit(cmd.Context(), output.Ok(cmd.OutOrStdout(), map[string]any{"paused": true}))
			return nil
		},
	}))

	var out bytes.Buffer
	code := clihelp.Execute(parent, &out, []string{"paused"})

	if code != 0 {
		t.Fatalf("exit code = %d; want 0, output: %s", code, out.String())
	}
	if notes := refusalNotes(t, dir); len(notes) != 0 {
		t.Errorf("an ok envelope wrote notes %v; want none", notes)
	}
}

func TestAddVerbs_ValidateRefusal_WritesNoNote(t *testing.T) {
	dir := t.TempDir()
	c, _ := newTestCLI(t)
	c.frictionDir = dir

	code, out := runVerbs(c, true, "validate")

	if code != 1 || !strings.Contains(out, `"ok":false`) {
		t.Fatalf("validate on a missing plan = (%d, %q); want a refusal with exit 1", code, out)
	}
	if notes := refusalNotes(t, dir); len(notes) != 0 {
		t.Errorf("validate wrote notes %v; want none", notes)
	}
}

func TestNoteRefusals_EmptyFrictionDir_WritesNothingAndKeepsEnvelope(t *testing.T) {
	c, _ := newTestCLI(t)
	args := []string{"begin-batch", "x"}

	rawCode, rawOut := runVerbs(c, false, args...)
	code, out := runVerbs(c, true, args...)

	if code != rawCode || out != rawOut {
		t.Errorf("noted run = (%d, %q); want the unwrapped (%d, %q)", code, out, rawCode, rawOut)
	}
}

func TestNoteRefusals_UnwritableFrictionDir_KeepsExitAndEnvelope(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", blocker, err)
	}
	c, _ := newTestCLI(t)
	c.frictionDir = blocker
	args := []string{"begin-batch", "x"}

	rawCode, rawOut := runVerbs(c, false, args...)
	code, out := runVerbs(c, true, args...)

	if code != rawCode || out != rawOut {
		t.Errorf("noted run = (%d, %q); want the unwrapped (%d, %q)", code, out, rawCode, rawOut)
	}
}
