// cli_test.go covers the parts of `lyx gate test` that spawn nothing: the refusals that precede hub resolution and the slot wait its help text names.

package gatecli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestGateTest_RefusesBeforeResolvingTheHub(t *testing.T) {
	t.Parallel()

	missingDir := filepath.Join(t.TempDir(), "absent")
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{name: "--tags names --tier", args: []string{"test", "--tags", "integration", "./pkg"}, wantError: "--tier"},
		{name: "-C with --all names explicit targets", args: []string{"test", "-C", "sub", "--all"}, wantError: "explicit targets"},
		{name: "-C with a computed scope names explicit targets", args: []string{"test", "-C", "sub"}, wantError: "explicit targets"},
		{name: "-C with --card names explicit targets", args: []string{"test", "-C", "sub", "--card", "1"}, wantError: "explicit targets"},
		{name: "--all with explicit targets is two scope forms", args: []string{"test", "--all", "./pkg"}, wantError: "one scope form"},
		{name: "--all with --base is two scope forms", args: []string{"test", "--all", "--base", "main"}, wantError: "one scope form"},
		{name: "-C directory does not exist", args: []string{"test", "-C", missingDir, "./pkg"}, wantError: "does not exist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := RunCLIIn(t.TempDir(), &out, tc.args)

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("output %q is not a JSON envelope: %v", out.String(), err)
			}
			if code != 1 || envelope.OK || !strings.Contains(envelope.Error, tc.wantError) || !strings.Contains(envelope.Error, "way forward") {
				t.Errorf("RunCLIIn(%v) = %d, %+v; want exit 1, ok false and an error holding %q and a way forward", tc.args, code, envelope, tc.wantError)
			}
		})
	}

	t.Run("help names the shared keep-cache-warm cap as the slot wait", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		RunCLI(&out, []string{"test", "--help"})
		want := "for up to " + shuttleengine.DefaultAwaitCap.String()
		if !strings.Contains(out.String(), want) {
			t.Errorf("RunCLI(test --help) output %q lacks %q", out.String(), want)
		}
	})
}
