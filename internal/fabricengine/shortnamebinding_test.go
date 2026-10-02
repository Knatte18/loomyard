// shortnamebinding_test.go covers the .lyx-shortname record's read/write helpers and every row of resolveEffectiveShortname.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortnameRecord_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteShortname(dir, "lx"); err != nil {
		t.Fatalf("WriteShortname() error = %v; want nil", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ShortnameFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "lx\n" {
		t.Errorf("record = %q; want %q", raw, "lx\n")
	}
	if shortname, found := ReadShortname(dir); !found || shortname != "lx" {
		t.Errorf("ReadShortname() = %q, %v; want %q, true", shortname, found, "lx")
	}
}

func TestReadShortname_NotFound(t *testing.T) {
	tests := []struct {
		name    string
		content *string
	}{
		{name: "absent"},
		{name: "blank", content: ptr("  \n")},
		{name: "grammar", content: ptr("LX-1\n")},
		{name: "too short", content: ptr("a\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != nil {
				if err := os.WriteFile(filepath.Join(dir, ShortnameFileName), []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if shortname, found := ReadShortname(dir); found || shortname != "" {
				t.Errorf("ReadShortname() = %q, %v; want empty, false", shortname, found)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestResolveEffectiveShortname(t *testing.T) {
	tests := []struct {
		name        string
		recorded    string
		found       bool
		supplied    string
		freshBind   bool
		wantEff     string
		wantWrite   bool
		wantWarnSub string
		wantErrSubs []string
	}{
		{name: "fresh without shortname refuses", freshBind: true, wantErrSubs: []string{"--shortname"}},
		{name: "fresh with shortname records", freshBind: true, supplied: "lx", wantEff: "lx", wantWrite: true},
		{name: "record present derives", recorded: "lx", found: true, wantEff: "lx"},
		{name: "record present equal shortname", recorded: "lx", found: true, supplied: "lx", wantEff: "lx"},
		{name: "record present differing shortname refuses", recorded: "lx", found: true, supplied: "ly", wantErrSubs: []string{`"lx"`, "lyx fabric shortname"}},
		{name: "bound without record takes shortname", supplied: "lx", wantEff: "lx", wantWrite: true},
		{name: "bound without record or shortname warns", wantWarnSub: "lyx fabric shortname"},
		{name: "invalid supplied shortname refuses first", recorded: "lx", found: true, supplied: "L", wantErrSubs: []string{"invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eff, write, warning, err := resolveEffectiveShortname(tt.recorded, tt.found, tt.supplied, tt.freshBind)
			if len(tt.wantErrSubs) > 0 {
				if err == nil {
					t.Fatalf("err = nil; want a refusal containing %v", tt.wantErrSubs)
				}
				for _, sub := range tt.wantErrSubs {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("err = %q; want it to contain %q", err.Error(), sub)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v; want nil", err)
			}
			if eff != tt.wantEff || write != tt.wantWrite {
				t.Errorf("got (%q, %v); want (%q, %v)", eff, write, tt.wantEff, tt.wantWrite)
			}
			if tt.wantWarnSub == "" && warning != "" {
				t.Errorf("warning = %q; want none", warning)
			}
			if tt.wantWarnSub != "" && !strings.Contains(warning, tt.wantWarnSub) {
				t.Errorf("warning = %q; want it to contain %q", warning, tt.wantWarnSub)
			}
		})
	}
}
