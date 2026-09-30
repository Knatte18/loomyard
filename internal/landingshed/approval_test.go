package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApproval_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "approval.json")
	want := Approval{PRNumber: 7, HeadSHA: "abc123", ApprovedAt: "2026-01-02T03:04:05Z"}
	if err := WriteApproval(path, want); err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}
	got, found, err := ReadApproval(path)
	if err != nil || !found || got != want {
		t.Fatalf("ReadApproval = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}
}

func TestApproval_SecondWriteOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approval.json")
	first := Approval{PRNumber: 1, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}
	second := Approval{PRNumber: 2, HeadSHA: "bbb", ApprovedAt: "2026-01-02T00:00:00Z"}
	for _, a := range []Approval{first, second} {
		if err := WriteApproval(path, a); err != nil {
			t.Fatalf("WriteApproval: %v", err)
		}
	}
	got, found, err := ReadApproval(path)
	if err != nil || !found || got != second {
		t.Fatalf("ReadApproval = %+v, %v, %v; want %+v", got, found, err, second)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want 1 (no temp file left)", len(entries))
	}
}

func TestReadApproval_Absent(t *testing.T) {
	_, found, err := ReadApproval(filepath.Join(t.TempDir(), "none.json"))
	if found || err != nil {
		t.Fatalf("found=%v err=%v; want false, nil", found, err)
	}
}

func TestReadApproval_Invalid(t *testing.T) {
	cases := map[string]string{
		"malformed":      `{not json`,
		"empty head_sha": `{"pr_number":1,"head_sha":"","approved_at":"2026-01-01T00:00:00Z"}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "approval.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadApproval(path); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}
