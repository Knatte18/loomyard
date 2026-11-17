// arbiternote_test.go covers ArbiterNote's naming of each implementer's run files under an anchor.

package weaverengine_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestArbiterNote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	anchor := t.TempDir()
	weaverDir := weaverengine.Dir(anchor)
	websterDir := websterengine.Dir(anchor)
	tests := []struct {
		name        string
		implementer weaverengine.Implementer
		want        []string
		wantAbsent  []string
	}{
		{
			name:        "Weaver",
			implementer: weaverengine.ImplementerWeaver,
			want: []string{
				planparser.PlanDir(anchor),
				filepath.Join(weaverDir, "state.json"),
				weaverengine.ReportsDir(anchor),
				filepath.Join(weaverDir, "journal.md"),
			},
			wantAbsent: []string{websterDir},
		},
		{
			name:        "Webster",
			implementer: weaverengine.ImplementerWebster,
			want:        []string{websterDir, filepath.Join(websterDir, "state.json")},
			wantAbsent:  []string{weaverDir},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := weaverengine.ArbiterNote(tt.implementer, anchor)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("ArbiterNote(%s) = %q; want it to name %q", tt.implementer, got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("ArbiterNote(%s) = %q; want it not to name %q", tt.implementer, got, absent)
				}
			}
		})
	}
}
