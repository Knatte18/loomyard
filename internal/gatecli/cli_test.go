// cli_test.go covers the parts of `lyx gate test` that spawn nothing: the refusals that precede hub resolution.

package gatecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateTest_RefusesBeforeResolvingTheHub(t *testing.T) {
	t.Parallel()

	missingDir := filepath.Join(t.TempDir(), "absent")
	tests := []struct {
		name string
		// dirs are created under the row's own cwd before the run.
		dirs      []string
		args      []string
		wantError string
		// wantAbsent is text the error must not contain.
		wantAbsent string
	}{
		{name: "no package", args: []string{"test"}, wantError: "name at least one package"},
		{name: "only go test flags", args: []string{"test", "--", "-run", "X"}, wantError: "name at least one package"},
		{name: "-C directory does not exist", args: []string{"test", "-C", missingDir, "./pkg"}, wantError: "does not exist"},
		{name: "external test package names its package directory", dirs: []string{"internal/x"}, args: []string{"test", "./internal/x_test"}, wantError: "./internal/x;"},
		{name: "only the missing argument is named", dirs: []string{"internal/x"}, args: []string{"test", "./nope", "./internal/x"}, wantError: "./nope", wantAbsent: "package argument ./internal/x "},
		{name: "-C arguments resolve against that directory", dirs: []string{"sub/x"}, args: []string{"test", "-C", "sub", "./x_test"}, wantError: "pass ./x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cwd := t.TempDir()
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(cwd, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, tc.args)

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
			if tc.wantAbsent != "" && strings.Contains(envelope.Error, tc.wantAbsent) {
				t.Errorf("RunCLIIn(%v) error %q holds %q", tc.args, envelope.Error, tc.wantAbsent)
			}
		})
	}
}
