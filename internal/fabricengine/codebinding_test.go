// codebinding_test.go covers the .lyx-code record's read/write helpers and every row of
// resolveEffectiveCode.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeRecord_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteCode(dir, "lx"); err != nil {
		t.Fatalf("WriteCode() error = %v; want nil", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, CodeFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "lx\n" {
		t.Errorf("record = %q; want %q", raw, "lx\n")
	}
	if code, found := ReadCode(dir); !found || code != "lx" {
		t.Errorf("ReadCode() = %q, %v; want %q, true", code, found, "lx")
	}
}

func TestReadCode_NotFound(t *testing.T) {
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
				if err := os.WriteFile(filepath.Join(dir, CodeFileName), []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if code, found := ReadCode(dir); found || code != "" {
				t.Errorf("ReadCode() = %q, %v; want empty, false", code, found)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestResolveEffectiveCode(t *testing.T) {
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
		{name: "fresh without code refuses", freshBind: true, wantErrSubs: []string{"--code"}},
		{name: "fresh with code records", freshBind: true, supplied: "lx", wantEff: "lx", wantWrite: true},
		{name: "record present derives", recorded: "lx", found: true, wantEff: "lx"},
		{name: "record present equal code", recorded: "lx", found: true, supplied: "lx", wantEff: "lx"},
		{name: "record present differing code refuses", recorded: "lx", found: true, supplied: "ly", wantErrSubs: []string{`"lx"`, "lyx fabric code"}},
		{name: "bound without record takes code", supplied: "lx", wantEff: "lx", wantWrite: true},
		{name: "bound without record or code warns", wantWarnSub: "lyx fabric code"},
		{name: "invalid supplied code refuses first", recorded: "lx", found: true, supplied: "L", wantErrSubs: []string{"invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eff, write, warning, err := resolveEffectiveCode(tt.recorded, tt.found, tt.supplied, tt.freshBind)
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
