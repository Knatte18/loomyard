package landingshed

import (
	"os"
	"path/filepath"
	"testing"
)

func validRejection() Rejection {
	return Rejection{PRNumber: 7, HeadSHA: "abc123", RejectedAt: "2026-01-02T03:04:05Z", Findings: "fix the thing"}
}

func TestRejection_RoundTripAndOverwrite(t *testing.T) {
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
}

func TestReadRejection_Absent(t *testing.T) {
	_, found, err := ReadRejection(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestReadRejection_Invalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRejection(path); err == nil {
		t.Fatal("want error")
	}
}

func TestReadRejection_MissingFields(t *testing.T) {
	cases := map[string]func(*Rejection){
		"pr":       func(r *Rejection) { r.PRNumber = 0 },
		"sha":      func(r *Rejection) { r.HeadSHA = "" },
		"at":       func(r *Rejection) { r.RejectedAt = "" },
		"findings": func(r *Rejection) { r.Findings = "" },
		"blank":    func(r *Rejection) { r.Findings = " \n\t" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "r.json")
			r := validRejection()
			mutate(&r)
			if err := WriteRejection(path, r); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadRejection(path); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestRemoveRecord(t *testing.T) {
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
