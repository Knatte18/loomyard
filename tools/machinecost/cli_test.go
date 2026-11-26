// cli_test.go covers the command line: each subcommand's exit code, and that compare reads the report probe writes.

package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
)

func TestWriteReport_ReadsBackUnchanged(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	path := filepath.Join(t.TempDir(), "report.json")
	want := Report{
		GeneratedAt: "2026-11-26T12:00:00Z",
		Machine:     Machine{Hostname: "laptop", Cores: 16, TrapsPresent: true},
		Before:      Snapshot{Load1: ptr(2.93)},
		Config:      ProbeConfig{Dirs: []NamedDir{{Name: "zz", Path: "/tmp/zz-probe"}}, Starts: 200},
		Skipped:     []string{"start/lym_help: no lym on PATH and no -lym flag"},
		Measurements: []Measurement{
			{Name: "newbinary/zz/first_run_ms", Unit: unitMilliseconds, Value: 3012.5},
		},
	}
	if err := writeReport(path, want); err != nil {
		t.Fatalf("writeReport() error: %v", err)
	}
	got, err := readReport(path)
	if err != nil {
		t.Fatalf("readReport() error: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("readReport(writeReport()) mismatch (-want +got):\n%s", diff)
	}
}

func TestRun_ExitCodes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	if err := writeReport(reportPath, Report{Measurements: []Measurement{{Name: "start/true/median_us", Unit: unitMicroseconds, Value: 1300}}}); err != nil {
		t.Fatalf("writeReport() error: %v", err)
	}
	missingPath := filepath.Join(dir, "missing.json")
	// wantStderr, where set, tells which of the refusals sharing an exit code fired.
	tests := []struct {
		name       string
		args       []string
		want       int
		wantStderr string
	}{
		{"NoSubcommand", nil, exitUsage, ""},
		{"UnknownSubcommand", []string{"measure"}, exitUsage, ""},
		{"CompareTwoReports", []string{"compare", reportPath, reportPath}, exitOK, ""},
		{"CompareOneReport", []string{"compare", reportPath}, exitUsage, ""},
		{"CompareMissingReport", []string{"compare", reportPath, missingPath}, exitError, ""},
		{"ProbeZeroCount", []string{"probe", "-starts", "0"}, exitUsage, ""},
		{"ProbeUnknownFlag", []string{"probe", "-rounds", "3"}, exitUsage, ""},
		{"ProbeStrayArgument", []string{"probe", "extra"}, exitUsage, ""},
		{"ProbeDirNameGivenTwice", []string{"probe", "-dir", "a=/x", "-dir", "a=/y"}, exitUsage,
			`folder name "a" given twice`},
		{"ProbeDirIsAccepted", []string{"probe", "-dir", "a=/x", "-starts", "0"}, exitUsage,
			"every count must be at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			if got := run(tt.args, io.Discard, &stderr); got != tt.want {
				t.Errorf("run(%q) = %d; want %d", tt.args, got, tt.want)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("run(%q) stderr = %q; want it to hold %q", tt.args, stderr.String(), tt.wantStderr)
			}
		})
	}
}
