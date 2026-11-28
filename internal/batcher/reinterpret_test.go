// reinterpret_test.go covers RetireActive over hub batcher.yaml bytes.

package batcher_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRetireActive pins the outcomes of the rewrite.
// An active key is removed whatever it named, the rest of the file kept, and the removal reported with the keys
// that choose a profile now; it is never flagged.
// A file with no active key is returned unchanged, and a file that does not parse is an error.
func TestRetireActive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const instead = "name each implementer's profile in its own config instead, webster.batch_profile and weaver.batch_profile, " +
		"set from the prime with `lym config <module> --layer default --set batch_profile=<profile>`"
	tests := []struct {
		name        string
		input       string
		want        string
		wantRewrote []string
		wantErr     bool
	}{
		{
			name:        "ActiveBesideProfilesIsRemoved",
			input:       "profiles: {}\nactive: wide\n",
			want:        "profiles: {}\n",
			wantRewrote: []string{`removed batcher.active, which named "wide"; ` + instead},
		},
		{
			name:        "ActiveNamingAnOperatorProfileIsRemovedAndKeepsTheProfile",
			input:       "active: mine\nprofiles:\n  mine:\n    batchifier: identity\n",
			want:        "profiles:\n  mine:\n    batchifier: identity\n",
			wantRewrote: []string{`removed batcher.active, which named "mine"; ` + instead},
		},
		{
			name:  "NoActiveKeyIsUnchanged",
			input: "profiles:\n  wide:\n    budget: 250000\n",
			want:  "profiles:\n  wide:\n    budget: 250000\n",
		},
		{
			name:  "EmptyFileIsUnchanged",
			input: "",
			want:  "",
		},
		{
			name:    "UnparsableFileErrors",
			input:   "profiles: [not valid yaml\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := batcher.RetireActive([]byte(tt.input), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RetireActive(%q) = nil error; want a parse error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("RetireActive(%q) error = %v; want nil", tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("RetireActive(%q) = %q; want %q", tt.input, got, tt.want)
			}
			if diff := cmp.Diff(tt.wantRewrote, rewrote); diff != "" {
				t.Errorf("RetireActive() rewrote mismatch (-want +got):\n%s", diff)
			}
			if len(flagged) != 0 {
				t.Errorf("RetireActive() flagged = %v; want none", flagged)
			}
		})
	}
}
