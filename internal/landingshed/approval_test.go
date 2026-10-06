// approval_test.go — untagged Tier-1 unit tests for WriteApproval and ReadApproval over a
// t.TempDir() record.

package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApproval_RoundTripAndOverwrite pins every Approval field surviving the file, a second write replacing the first, and no temp file left behind.
//
//testtiming:keep round-trips each Approval field, the overwrite and the absence of a temp file through the real file, which the PRGate tests covering its blocks never read back
func TestApproval_RoundTripAndOverwrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sub", "approval.json")
	first := Approval{PRNumber: 1, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}
	second := Approval{PRNumber: 7, HeadSHA: "abc123", ApprovedAt: "2026-01-02T03:04:05Z"}
	for _, want := range []Approval{first, second} {
		if err := WriteApproval(path, want); err != nil {
			t.Fatalf("WriteApproval: %v", err)
		}
		got, found, err := ReadApproval(path)
		if err != nil || !found || got != want {
			t.Fatalf("ReadApproval = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want 1 (no temp file left)", len(entries))
	}
}

func TestReadApproval_AbsentOrInvalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		content   string
		wantFound bool
		wantErr   bool
	}{
		{"absent", "", false, false},
		{"malformed", `{not json`, false, true},
		{"empty head_sha", `{"pr_number":1,"head_sha":"","approved_at":"2026-01-01T00:00:00Z"}`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "approval.json")
			if tc.content != "" {
				if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, found, err := ReadApproval(path)
			if found != tc.wantFound || (err != nil) != tc.wantErr {
				t.Fatalf("found=%v err=%v; want found=%v, error=%v", found, err, tc.wantFound, tc.wantErr)
			}
		})
	}
}
