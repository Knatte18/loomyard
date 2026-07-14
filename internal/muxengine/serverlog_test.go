// serverlog_test.go verifies the pure debug_log parsing helper. Untagged: no
// filesystem or process I/O, per Test Tier Purity.

package muxengine

import (
	"reflect"
	"testing"
)

func TestDebugLogArgs(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    []string
		wantErr bool
	}{
		{"off", "0", nil, false},
		{"verbose", "1", []string{"-v"}, false},
		{"veryVerbose", "2", []string{"-vv"}, false},
		{"whitespaceTrimmed", " 1 ", []string{"-v"}, false},
		{"empty", "", nil, true},
		{"invalid", "3", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := debugLogArgs(tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("debugLogArgs(%q) = %v, nil; want error", tt.level, got)
				}
				wantMsg := `invalid debug_log "` + tt.level + `": must be 0, 1 or 2`
				if err.Error() != wantMsg {
					t.Errorf("debugLogArgs(%q) error = %q; want %q", tt.level, err.Error(), wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("debugLogArgs(%q) unexpected error: %v", tt.level, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("debugLogArgs(%q) = %v; want %v", tt.level, got, tt.want)
			}
		})
	}
}
