// legacybeam_test.go pins BeamSettingsByEntry.
// It reads each entry's beam settings from board.json for the hub reconcile.

package boardengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBeamSettingsByEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// body is board.json; empty leaves the file absent.
		body string
		want map[string]boardengine.EntryBeamSettings
	}{
		{
			name: "a task's retired keys carry as its beam settings",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","pull_request":"merge-in","notify_on_land":true}]}`,
			want: map[string]boardengine.EntryBeamSettings{"a": {Set: map[string]string{"pull_request": "merge-in", "land_notice": "on"}}},
		},
		{
			name: "a project's notify_on_land carries too",
			body: `{"version":1,"entries":[{"id":0,"slug":"p","title":"P","kind":"project","notify_on_land":true}]}`,
			want: map[string]boardengine.EntryBeamSettings{"p": {Set: map[string]string{"land_notice": "on"}}},
		},
		{
			name: "a retired pull_request of off is reported as retired off, and a notify_on_land of false carries nothing",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","pull_request":"off","notify_on_land":false}]}`,
			want: map[string]boardengine.EntryBeamSettings{"a": {Set: map[string]string{}, RetiredOff: true}},
		},
		{
			name: "the beam run settings carry as the entry's settings",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","run_settings":{"beam":{"pull_request":"off","land_notice":"on"}}}]}`,
			want: map[string]boardengine.EntryBeamSettings{"a": {Set: map[string]string{"pull_request": "off", "land_notice": "on"}}},
		},
		{
			name: "a run setting wins over a retired key, and then no retired off is reported",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","pull_request":"off","notify_on_land":true,` +
				`"run_settings":{"beam":{"pull_request":"merge"}}}]}`,
			want: map[string]boardengine.EntryBeamSettings{"a": {Set: map[string]string{"pull_request": "merge", "land_notice": "on"}}},
		},
		{
			name: "an entry with no setting is left out",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","run_settings":{"loom":{"x":1}}}]}`,
			want: map[string]boardengine.EntryBeamSettings{},
		},
		{
			name: "an absent board.json gives none",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			if tt.body != "" {
				if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(tt.body), 0o644); err != nil {
					t.Fatalf("write board.json: %v", err)
				}
			}

			got, err := boardengine.BeamSettingsByEntry(boardDir)
			if err != nil {
				t.Fatalf("BeamSettingsByEntry() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BeamSettingsByEntry() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
