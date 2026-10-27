//go:build tier0

// tiersleepforms_test.go pins the long-sleep detection tiersleep_test.go holds against crafted code snippets, one per argument form.

package main

import (
	"go/token"
	"testing"
)

// TestFindLongLiteralSleep_DetectsAllArgumentForms verifies detection against crafted code snippets.
//
//testtiming:keep proves the sleep check of TestTierPurity_UntaggedTestsSpawnNothing fires on every argument form of a long literal sleep
func TestFindLongLiteralSleep_DetectsAllArgumentForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "bare_scale_at_least_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Hour) }\n",
			want: true,
		},
		{
			name: "bare_scale_under_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Millisecond) }\n",
			want: false,
		},
		{
			name: "multiplication_at_least_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(2 * time.Second) }\n",
			want: true,
		},
		{
			name: "multiplication_under_one_second",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(500 * time.Millisecond) }\n",
			want: false,
		},
		{
			name: "named_const_resolved_by_identifier",
			src:  "package p\nimport \"time\"\nconst waitLong = time.Hour\nfunc f() { time.Sleep(waitLong) }\n",
			want: true,
		},
		{
			name: "identifier_with_no_resolvable_declaration",
			src:  "package p\nimport \"time\"\nfunc f() { time.Sleep(undeclaredDuration) }\n",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			_, found := findLongLiteralSleep(fset, tt.name+".go", []byte(tt.src))
			if found != tt.want {
				t.Errorf("findLongLiteralSleep(%s) found = %v; want %v", tt.name, found, tt.want)
			}
		})
	}
}
