package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

func validRejection() Rejection {
	return Rejection{PRNumber: 7, HeadSHA: "abc123", RejectedAt: "2026-01-02T03:04:05Z", Findings: "fix the thing"}
}

// TestRejection_RoundTripAndOverwrite pins every Rejection field surviving the file, and a second write replacing the first.
//
//testtiming:keep round-trips each Rejection field and the overwrite through the real file, which the PRGate tests covering its blocks never read back
func TestRejection_RoundTripAndOverwrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sub", "rejection.json")
	want := validRejection()
	if err := WriteRejection(path, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := ReadRejection(path)
	if err != nil || !found || got != want {
		t.Fatalf("got %+v found=%v err=%v", got, found, err)
	}
	want.Findings = "second"
	if err := WriteRejection(path, want); err != nil {
		t.Fatal(err)
	}
	got, _, err = ReadRejection(path)
	if err != nil || got != want {
		t.Fatalf("overwrite: got %+v err=%v", got, err)
	}

	t.Run("RecordRejection removes a pending approval and writes the rejection", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		approvalPath, rejectionPath := filepath.Join(dir, "approval.json"), filepath.Join(dir, "rejection.json")
		if err := WriteApproval(approvalPath, Approval{PRNumber: 7, HeadSHA: "abc123", ApprovedAt: "2026-01-02T03:04:05Z"}); err != nil {
			t.Fatal(err)
		}
		if err := RecordRejection(approvalPath, rejectionPath, want); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(approvalPath); !os.IsNotExist(err) {
			t.Errorf("approval still present: %v", err)
		}
		if got, found, err := ReadRejection(rejectionPath); err != nil || !found || got != want {
			t.Errorf("ReadRejection = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
		}
	})

	t.Run("RecordRejection writes no rejection when the approval cannot be removed", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		approvalPath, rejectionPath := filepath.Join(dir, "approval.json"), filepath.Join(dir, "rejection.json")
		if err := os.MkdirAll(filepath.Join(approvalPath, "held"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := RecordRejection(approvalPath, rejectionPath, want); err == nil {
			t.Fatal("RecordRejection = nil; want the removal's error")
		}
		if _, err := os.Stat(rejectionPath); !os.IsNotExist(err) {
			t.Errorf("rejection written despite the failed removal: %v", err)
		}
	})
}

func TestReadRejection_AbsentOrInvalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		write   func(path string) error
		wantErr bool
	}{
		{"absent", func(string) error { return nil }, false},
		{"malformed", func(path string) error { return os.WriteFile(path, []byte("{nope"), 0o644) }, true},
		{"missing pr", writeMutatedRejection(func(r *Rejection) { r.PRNumber = 0 }), true},
		{"missing sha", writeMutatedRejection(func(r *Rejection) { r.HeadSHA = "" }), true},
		{"missing rejected_at", writeMutatedRejection(func(r *Rejection) { r.RejectedAt = "" }), true},
		{"missing findings", writeMutatedRejection(func(r *Rejection) { r.Findings = "" }), true},
		{"blank findings", writeMutatedRejection(func(r *Rejection) { r.Findings = " \n\t" }), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "r.json")
			if err := tc.write(path); err != nil {
				t.Fatal(err)
			}
			_, found, err := ReadRejection(path)
			if found || (err != nil) != tc.wantErr {
				t.Fatalf("found=%v err=%v; want found=false, error=%v", found, err, tc.wantErr)
			}
		})
	}
}

// writeMutatedRejection returns a writer of validRejection with mutate applied, for a record that fails validation on read.
func writeMutatedRejection(mutate func(*Rejection)) func(path string) error {
	return func(path string) error {
		r := validRejection()
		mutate(&r)
		return WriteRejection(path, r)
	}
}

func TestRemoveRecord(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "r.json")
	if err := WriteRejection(path, validRejection()); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRecord(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("record still present: %v", err)
	}
	if err := RemoveRecord(path); err != nil {
		t.Fatalf("absent: %v", err)
	}
}
