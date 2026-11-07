// main_test.go checks tokencount's flag set.
// It refuses the retired history and projects flags; a foreign hub without a records repository;
// a non-date -since, and a -last below 1 without -since.
// A -since date parses to the start of its local day, and the hub flag resolves against the current directory.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestResolveHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	wd := filepath.Join(string(filepath.Separator), "hubs", "main", "prime")
	parent := filepath.Dir(wd)
	other := filepath.Join(string(filepath.Separator), "hubs", "other")
	tests := []struct {
		name    string
		hub     string
		records string
		want    string
		wantErr bool
	}{
		{"empty is the parent", "", "", parent, false},
		{"parent with a trailing slash", parent + string(filepath.Separator), "", parent, false},
		{"relative parent", "..", "", parent, false},
		{"another directory without a records", other, "", "", true},
		{"another directory with a records", other, "records", other, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveHub(tt.hub, wd, tt.records)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveHub(%q, %q, %q) error = %v; want error = %v", tt.hub, wd, tt.records, err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), "-"+repositoryFlag) || !strings.Contains(err.Error(), tt.hub) {
					t.Errorf("resolveHub error = %q; want it to name the hub %q and the records flag", err, tt.hub)
				}
				return
			}
			if got != tt.want {
				t.Errorf("resolveHub(%q, %q, %q) = %q; want %q", tt.hub, wd, tt.records, got, tt.want)
			}
		})
	}
}

func TestParseSince(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	got, err := parseSince("2026-01-02")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("parseSince(2026-01-02) = %v; want %v", got, want)
	}
	got, err = parseSince("")
	if err != nil || !got.IsZero() {
		t.Errorf("parseSince(\"\") = %v, %v; want the zero time and no error", got, err)
	}
}

func TestRunRefuses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"the retired history flag", []string{"-history", "somewhere", "-calibrate", "fit", "slug"}, "flag provided but not defined: -history"},
		{"the retired projects flag", []string{"-projects", "somewhere", "slug"}, "flag provided but not defined: -projects"},
		{"a foreign hub without a records", []string{"-hub", t.TempDir(), "slug"}, "-" + repositoryFlag},
		{"a since that is no date", []string{"-since", "yesterday"}, `-since "yesterday" is not a YYYY-MM-DD date`},
		{"a last of zero without since", []string{"-last", "0"}, "-last 0 counts no run"},
		{"a negative last without since", []string{"-last", "-1"}, "-last -1 counts no run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := run(tt.args, &out, ReadSet{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run error = %v; want one containing %q", err, tt.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("run wrote %q; want no report", out.String())
			}
		})
	}
}
