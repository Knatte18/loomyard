//go:build tier0

// report_test.go covers `lym gate report`: the grouping and totals of run records, the filters, the date refusal and the tolerant record reader.

package gatecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

func TestGateReport(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	records := []gateslot.RunRecord{
		{Slug: "s1", GateKey: "webster-fix", TierList: []int{0}, ScopeForm: "computed", WaitStart: at, Start: at.Add(2 * time.Second), WallSec: 10,
			Launches: []gateslot.PackageLaunch{{Package: "a", LaunchSeconds: 0.5, ElapsedSeconds: 2}, {Package: "b", LaunchSeconds: 0.5, ElapsedSeconds: 2}}},
		{Slug: "s1", GateKey: "webster-fix", TierList: []int{0}, ScopeForm: "computed", WaitStart: at, Start: at, Carried: true},
		{Slug: "s1", TierList: []int{0, 1}, ScopeForm: "all", Start: at, WallSec: 3},
		{Slug: "s2", GateKey: "finalize", TierList: []int{0}, ScopeForm: "all", WaitStart: at.Add(-time.Second), Start: at, WallSec: 5},
		{Slug: "s2", GateKey: "finalize", TierList: []int{0}, ScopeForm: "all", Start: at.Add(-72 * time.Hour), WallSec: 7},
		{Slug: "s3", GateKey: "batch", TierList: []int{0}, ScopeForm: "build", Build: true, WaitStart: at.Add(-time.Second), Start: at, WallSec: 4},
	}
	ptr := func(value float64) *float64 { return &value }
	type row struct {
		Slug          string   `json:"slug"`
		GateKey       string   `json:"gate_key"`
		TierRuns      int      `json:"tier_runs"`
		Carried       int      `json:"carried"`
		WaitSeconds   float64  `json:"wait_seconds"`
		WallSeconds   float64  `json:"wall_seconds"`
		LaunchSeconds *float64 `json:"launch_seconds"`
		LaunchShare   *float64 `json:"launch_share"`
		BuildRuns     int      `json:"build_runs"`
		BuildSeconds  float64  `json:"build_seconds"`
	}
	keyed := row{Slug: "s1", GateKey: "webster-fix", TierRuns: 1, Carried: 1, WaitSeconds: 2, WallSeconds: 10, LaunchSeconds: ptr(1), LaunchShare: ptr(0.25)}
	unkeyed := row{Slug: "s1", GateKey: "tiers 0,1 all", TierRuns: 1, WallSeconds: 3}
	finalize := row{Slug: "s2", GateKey: "finalize", TierRuns: 2, WaitSeconds: 1, WallSeconds: 12}
	// A build record counts in build_runs and build_seconds alone.
	build := row{Slug: "s3", GateKey: "batch", BuildRuns: 1, BuildSeconds: 4}

	tests := []struct {
		name  string
		slug  string
		since time.Time
		want  []row
	}{
		{"every group in slug then key order", "", time.Time{}, []row{unkeyed, keyed, finalize, build}},
		{"a slug keeps only that slug's groups", "s2", time.Time{}, []row{finalize}},
		{"a since drops a record started before it", "", at.Add(-24 * time.Hour), []row{unkeyed, keyed, {Slug: "s2", GateKey: "finalize", TierRuns: 1, WaitSeconds: 1, WallSeconds: 5}, build}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := runReport(&out, records, tt.slug, tt.since); code != 0 {
				t.Fatalf("runReport() = %d, stdout %q; want 0", code, out.String())
			}
			var envelope struct {
				OK   bool  `json:"ok"`
				Rows []row `json:"rows"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("stdout %q is not a JSON envelope: %v", out.String(), err)
			}
			if !envelope.OK || !reflect.DeepEqual(envelope.Rows, tt.want) {
				t.Errorf("runReport() rows = %+v, ok %v; want %+v", envelope.Rows, envelope.OK, tt.want)
			}
		})
	}

	t.Run("a malformed --since is refused naming the expected form", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		if code := RunCLI(&out, []string{"report", "--since", "24-10-2026"}); code != 1 || !strings.Contains(out.String(), "YYYY-MM-DD") || !strings.Contains(out.String(), "way forward") {
			t.Errorf("RunCLI(report --since 24-10-2026) = %d, stdout %q; want exit 1 naming YYYY-MM-DD and a way forward", code, out.String())
		}
	})

	t.Run("the reader returns no records for a missing directory and skips a file that does not decode", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := gateslot.ReadRunRecords(filepath.Join(dir, "absent")); got != nil || err != nil {
			t.Errorf("ReadRunRecords(absent) = %v, %v; want no records and no error", got, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"slug":"s1","tier":0}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{not json`), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := gateslot.ReadRunRecords(dir)
		if err != nil || len(got) != 1 || got[0].Slug != "s1" {
			t.Errorf("ReadRunRecords(dir) = %+v, %v; want the one decodable record", got, err)
		}
	})
}
