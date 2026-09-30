//go:build integration

// verifygate_integration_test.go drives newVerifyGate's real shared runner with echo-and-exit
// commands and checks the output file holds the command's output on both a pass and a fail.

package landingshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyGate_RealRunner(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		wantReason bool
		wantOutput string
	}{
		{"pass", "echo verify-ok", false, "verify-ok"},
		{"fail", "echo verify-broken; exit 3", true, "verify-broken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			deps := Deps{
				WorktreeRoot:      root,
				VerifyCommand:     func() (string, error) { return tc.command, nil },
				VerifyPendingPath: filepath.Join(root, "scratch", "verify-pending"),
				VerifyOutputPath:  filepath.Join(root, "scratch", "verify-output.log"),
			}
			reason, err := newVerifyGate(deps).check(context.Background(), "Publish", "main", true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (reason != "") != tc.wantReason {
				t.Fatalf("reason = %q, wantReason = %v", reason, tc.wantReason)
			}
			got, err := os.ReadFile(deps.VerifyOutputPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), tc.wantOutput) {
				t.Fatalf("output file = %q, want it to contain %q", got, tc.wantOutput)
			}
		})
	}
}
