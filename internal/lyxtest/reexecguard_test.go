// reexecguard_test.go covers cliReexecArg's pure classification of a test binary's argv tail: only a LEADING non-flag argument marks a CLI-style re-exec;
// flag-led argv (including two-token flag values) stays allowed.

package lyxtest

import "testing"

func TestCLIReexecArg(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantArg string
		wantOK  bool
	}{
		{
			name:   "Empty",
			args:   nil,
			wantOK: false,
		},
		{
			name:    "HeaderPaneReexec",
			args:    []string{"reed", "header", "--blocking"},
			wantArg: "reed",
			wantOK:  true,
		},
		{
			name:   "GoTestFlagsOnly",
			args:   []string{"-test.run=TestFoo", "-test.v"},
			wantOK: false,
		},
		{
			name: "TwoTokenFlagValueAllowed",
			// A later positional-looking token can be a flag's value, so only
			// the leading argument is ever classified.
			args:   []string{"-test.run", "TestFoo"},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := cliReexecArg(tt.args)
			if arg != tt.wantArg || ok != tt.wantOK {
				t.Errorf("cliReexecArg(%q) = (%q, %v), want (%q, %v)", tt.args, arg, ok, tt.wantArg, tt.wantOK)
			}
		})
	}
}
