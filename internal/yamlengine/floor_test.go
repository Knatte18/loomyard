// floor_test.go pins RaiseIntFloor over every value shape a stored key can hold.

package yamlengine

import "testing"

// TestRaiseIntFloor pins which stored values are raised and which come back byte-identical, the reported line, and that comments and sibling keys survive a rewrite.
func TestRaiseIntFloor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existing      string
		wantOut       string
		wantLine      string
		wantErr       bool
		wantUnchanged bool
	}{
		{name: "below floor is raised", existing: "poll: 500\n", wantOut: "poll: 1000\n", wantLine: "poll: raised from 500 to 1000"},
		{name: "at floor is unchanged", existing: "poll: 1000\n", wantUnchanged: true},
		{name: "above floor is unchanged", existing: "poll: 2500\n", wantUnchanged: true},
		{name: "absent key is unchanged", existing: "other: 1\n", wantUnchanged: true},
		{name: "null tilde is raised", existing: "poll: ~\n", wantOut: "poll: 1000\n", wantLine: "poll: raised from null to 1000"},
		{name: "null word is raised", existing: "poll: null\n", wantOut: "poll: 1000\n", wantLine: "poll: raised from null to 1000"},
		{name: "empty value is raised", existing: "poll:\n", wantOut: "poll: 1000\n", wantLine: "poll: raised from null to 1000"},
		{name: "env marker is unchanged", existing: "poll: ${env:POLL:-5}\n", wantUnchanged: true},
		{name: "non-integer scalar is unchanged", existing: "poll: abc\n", wantUnchanged: true},
		{name: "empty document is unchanged", existing: "", wantUnchanged: true},
		{
			name:     "comments and sibling keys are preserved",
			existing: "# head\nname: x # trailing\npoll: 5 # ms\nlist:\n  - a\n  - b\n",
			wantOut:  "# head\nname: x # trailing\npoll: 1000 # ms\nlist:\n  - a\n  - b\n",
			wantLine: "poll: raised from 5 to 1000",
		},
		{name: "unparseable document errors", existing: "poll: [1\n", wantErr: true},
		{name: "non-mapping root errors", existing: "- 1\n- 2\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, line, err := RaiseIntFloor([]byte(tt.existing), "poll", 1000)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RaiseIntFloor() error = %v; want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			wantOut := tt.wantOut
			if tt.wantUnchanged {
				wantOut = tt.existing
			}
			if string(out) != wantOut || line != tt.wantLine {
				t.Errorf("RaiseIntFloor() = (%q, %q); want (%q, %q)", out, line, wantOut, tt.wantLine)
			}
		})
	}
}
