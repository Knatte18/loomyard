// cli_test.go covers the parts of `lyx gate test` that spawn nothing: the refusals that precede hub resolution and the go argument assembly.

package gatecli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGateTest_RefusesBeforeResolvingTheHub(t *testing.T) {
	t.Parallel()

	missingDir := filepath.Join(t.TempDir(), "absent")
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{"no package", []string{"test"}, "name at least one package"},
		{"only go test flags", []string{"test", "--", "-run", "X"}, "name at least one package"},
		{"-C directory does not exist", []string{"test", "-C", missingDir, "./pkg"}, "does not exist"},
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
}

func TestGoTestArgs_AssemblesDirCapTagsPackagesAndFlagsInOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tags     string
		packages []string
		flags    []string
		want     []string
	}{
		{"packages only", "", []string{"./a", "./b"}, nil, []string{"test", "-C", "/work", "-p", "3", "./a", "./b"}},
		{"tags and flags", "integration", []string{"./a"}, []string{"-run", "X"}, []string{"test", "-C", "/work", "-p", "3", "-tags", "integration", "./a", "-run", "X"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := goTestArgs("/work", 3, tc.tags, tc.packages, tc.flags); !slices.Equal(got, tc.want) {
				t.Errorf("goTestArgs = %q; want %q", got, tc.want)
			}
		})
	}
}
