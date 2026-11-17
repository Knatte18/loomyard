// hookedit_test.go pins the hidden hook-edit verb:
// the record it appends for an edit, its silence on standard output, and the exit code 0 it keeps on every failure.

package shuttlecli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHookEditVerb_RunsOutsideAHub pins that the verb skips hub resolution outside a git repository.
// Run through the shuttle command tree, the verb exits 0 and prints its own report.
// It prints no error envelope from hub resolution.
func TestHookEditVerb_RunsOutsideAHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), t.TempDir(), &out, []string{"hook-edit", "no-such-engine", "unused-edit-records-file"})
	if code != 0 || !strings.HasPrefix(out.String(), "shuttle: the engine") {
		t.Errorf("ExecuteIn(hook-edit) = exit %d, output %q; want exit 0 and the verb's own report", code, out.String())
	}
}

// TestHookEditVerb pins the verb's effect on the edit records file, its output and its exit code.
// An edit appends its record and prints nothing.
// An unknown engine or a malformed input is reported on standard error and appends nothing.
// The verb still returns no error.
func TestHookEditVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		engine string
		input  string
		// wantRecord is a text the appended record holds; empty wants nothing appended.
		wantRecord string
		// wantStderr is a text the standard error names; empty wants it empty.
		wantStderr string
	}{
		{name: "AnEditAppendsItsRecord", engine: "claude", input: `{"tool_name":"Edit","tool_input":{"file_path":"/w/a.go"}}`, wantRecord: `"file_path":"/w/a.go"`},
		{name: "UnknownEngineIsReportedAndExitsZero", engine: "no-such-engine", input: "{}", wantStderr: `the engine "no-such-engine" has no edit hook`},
		{name: "MalformedInputIsReportedAndExitsZero", engine: "claude", input: "{not json", wantStderr: "parse the edit hook input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recordsPath := filepath.Join(t.TempDir(), shuttleengine.EditRecordsFileName)
			if err := os.WriteFile(recordsPath, nil, 0o644); err != nil {
				t.Fatalf("seed the edit records file: %v", err)
			}

			cmd := hookEditCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetIn(strings.NewReader(tt.input))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{tt.engine, recordsPath})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error: %v; want nil on every path", err)
			}

			if stdout.Len() != 0 {
				t.Errorf("standard output = %q; want empty", stdout.String())
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Errorf("standard error = %q; want empty", stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("standard error = %q; want it to name %q", stderr.String(), tt.wantStderr)
			}
			data, err := os.ReadFile(recordsPath)
			if err != nil {
				t.Fatalf("read the edit records file: %v", err)
			}
			if tt.wantRecord == "" && len(data) != 0 {
				t.Errorf("edit records file = %q; want nothing appended", data)
			}
			if tt.wantRecord != "" && !strings.Contains(string(data), tt.wantRecord) {
				t.Errorf("edit records file = %q; want a record holding %s", data, tt.wantRecord)
			}
		})
	}
}
