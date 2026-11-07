// cli_test.go pins how `hibernate request` chooses whether its wake lifts the hub pause.
// The choice comes from its flag and the hub's hibernate.yaml.

package hibernatecli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChooseResumeOnWake(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		hubFile   string
		flagGiven bool
		flag      bool
		want      bool
		wantErr   bool
	}{
		{"no flag and no hub file read the template's off", "", false, false, false, false},
		{"no flag reads the hub file", "resume_on_wake: true\n", false, false, true, false},
		{"the flag on overrides a hub file that is off", "resume_on_wake: false\n", true, true, true, false},
		{"the flag off overrides a hub file that is on", "resume_on_wake: true\n", true, false, false, false},
		{"no flag and an unparsable hub file refuse", "resume_on_wake: [\n", false, false, false, true},
		{"the flag given wins over an unparsable hub file without reading it", "resume_on_wake: [\n", true, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := t.TempDir()
			if tt.hubFile != "" {
				path := configengine.ConfigFile(board, "hibernate")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.hubFile), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := chooseResumeOnWake(tt.flagGiven, tt.flag, configengine.Dirs{Board: board})

			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("chooseResumeOnWake(%v, %v) over %q = %v, %v; want %v, error %v", tt.flagGiven, tt.flag, tt.hubFile, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
