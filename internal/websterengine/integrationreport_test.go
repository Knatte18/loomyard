// integrationreport_test.go covers IntegrationReport's round trip, its fork-only shape, its strict
// decode and validation, and that the batch Report shape stays strict.

package websterengine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

func writeRaw(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "r.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIntegrationReport_RoundTrip(t *testing.T) {
	want := &websterengine.IntegrationReport{
		Status:     websterengine.ReportStatusFailed,
		HeadSHA:    "abc123",
		Deviations: []string{"a.go"},
		Failures: []websterengine.IntegrationFailure{
			{ID: "TestX", Kind: websterengine.FailureKindTest, Tail: "line1\nline2\n"},
			{ID: "pkg/y", Kind: websterengine.FailureKindPackage, Tail: "boom\n"},
		},
		Triage: &websterengine.IntegrationTriage{
			Verdict:     websterengine.TriageVerdictRegression,
			Rerun:       websterengine.TriageRerunFailed,
			BaselineSHA: "def456",
			Flaky:       []string{"TestF"},
			PreExisting: []string{"TestP"},
			Regressions: []string{"TestX"},
		},
	}
	path := filepath.Join(t.TempDir(), "r.yaml")
	if err := websterengine.WriteIntegrationReport(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := websterengine.ParseIntegrationReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v; want %+v", got, want)
	}
}

func TestIntegrationReport_ForkOnly(t *testing.T) {
	got, err := websterengine.ParseIntegrationReport(writeRaw(t, "status: OK\nhead_sha: abc\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Failures != nil || got.Triage != nil {
		t.Errorf("Failures=%v Triage=%v; want nil", got.Failures, got.Triage)
	}
}

func TestIntegrationReport_Rejects(t *testing.T) {
	cases := map[string]string{
		"unknown key": "status: OK\nhead_sha: abc\nbogus: 1\n",
		"bad status":  "status: MAYBE\nhead_sha: abc\n",
		"no head sha": "status: OK\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := websterengine.ParseIntegrationReport(writeRaw(t, body)); err == nil {
				t.Errorf("ParseIntegrationReport(%q) error = nil; want error", body)
			}
		})
	}
}

func TestParseReport_RejectsFailuresKey(t *testing.T) {
	path := writeRaw(t, "status: OK\nhead_sha: abc\nfailures:\n  - id: T\n    kind: test\n    tail: x\n")
	if _, err := websterengine.ParseReport(path); err == nil {
		t.Error("ParseReport with failures key error = nil; want error")
	}
}
