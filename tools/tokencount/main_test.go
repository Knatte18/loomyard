// main_test.go checks tokencount's flag set: the retired history and projects flags and a foreign hub without a records repository are refused, and the hub flag resolves against the current directory.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHub(t *testing.T) {
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

func TestRunRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"the retired history flag", []string{"-history", "somewhere", "-calibrate", "fit", "slug"}, "flag provided but not defined: -history"},
		{"the retired projects flag", []string{"-projects", "somewhere", "slug"}, "flag provided but not defined: -projects"},
		{"a foreign hub without a records", []string{"-hub", t.TempDir(), "slug"}, "-" + repositoryFlag},
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
