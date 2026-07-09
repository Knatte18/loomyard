// profile_test.go table-drives Profile.validate over the happy path and
// every fail-loud rule documented on validate: default resolution for
// RoundCaps/JudgeModel/JudgeEffort/Gate.Timeout, RoundCaps shape, Gate.Mode
// legality and its Command-emptiness pairing, and the two negative-duration
// rejections. It also separately exercises the three-level default
// resolution chain (profile > Config > built-in) for RoundCaps and
// JudgeModel.

package perchengine

import (
	"strings"
	"testing"
	"time"
)

// newValidProfile returns a Profile that passes validate unmodified against
// an empty Config — every test mutates a copy of this base to exercise one
// rule at a time. The embedded burler content fields are left zero-valued:
// validate does not check them.
func newValidProfile() Profile {
	return Profile{
		Gate: Gate{Mode: GateLLMVerdict},
	}
}

func TestProfile_Validate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(p *Profile)
		cfg       Config
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "valid profile",
			mutate: func(p *Profile) {},
		},
		{
			name: "one-element round-caps is a plain hard cap",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{10}
			},
		},
		{
			name: "command mode with argv",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: GateCommand, Command: []string{"go", "build", "./..."}}
			},
		},
		{
			name: "both mode with argv",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: GateBoth, Command: []string{"go", "vet", "./..."}}
			},
		},
		{
			name: "round-caps explicit empty list fails loud rather than defaulting",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{}
			},
			wantErr:   true,
			errSubstr: "must not be an explicit empty list",
		},
		{
			name: "round-caps non-positive entry",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{0, 5}
			},
			wantErr:   true,
			errSubstr: "profile.RoundCaps entries must all be >= 1",
		},
		{
			name: "round-caps negative entry",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{-1, 5}
			},
			wantErr:   true,
			errSubstr: "profile.RoundCaps entries must all be >= 1",
		},
		{
			name: "round-caps non-increasing",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{5, 5, 10}
			},
			wantErr:   true,
			errSubstr: "profile.RoundCaps must be strictly increasing",
		},
		{
			name: "round-caps decreasing",
			mutate: func(p *Profile) {
				p.RoundCaps = []int{8, 5, 10}
			},
			wantErr:   true,
			errSubstr: "profile.RoundCaps must be strictly increasing",
		},
		{
			name: "gate mode unset",
			mutate: func(p *Profile) {
				p.Gate = Gate{}
			},
			wantErr:   true,
			errSubstr: "profile.Gate.Mode must be",
		},
		{
			name: "gate mode unknown",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: "bogus"}
			},
			wantErr:   true,
			errSubstr: "profile.Gate.Mode must be",
		},
		{
			name: "llm-verdict with a non-empty command",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: GateLLMVerdict, Command: []string{"go", "test", "./..."}}
			},
			wantErr:   true,
			errSubstr: "must not set Gate.Command",
		},
		{
			name: "command mode with empty argv",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: GateCommand}
			},
			wantErr:   true,
			errSubstr: "requires a non-empty Gate.Command",
		},
		{
			name: "both mode with empty argv",
			mutate: func(p *Profile) {
				p.Gate = Gate{Mode: GateBoth}
			},
			wantErr:   true,
			errSubstr: "requires a non-empty Gate.Command",
		},
		{
			name: "negative gate timeout",
			mutate: func(p *Profile) {
				p.Gate.Timeout = -time.Second
			},
			wantErr:   true,
			errSubstr: "profile.Gate.Timeout must not be negative",
		},
		{
			name: "negative timeout",
			mutate: func(p *Profile) {
				p.Timeout = -time.Second
			},
			wantErr:   true,
			errSubstr: "profile.Timeout must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newValidProfile()
			tt.mutate(&p)
			err := p.validate(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() = nil; want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validate() = %q; want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
		})
	}
}

// TestProfile_Validate_RoundCapsDefaultChain exercises the three-level
// default resolution chain (profile > Config > built-in) in isolation: a
// profile value always wins, a Config value is used only when the profile
// is empty, and the built-in default is used only when both are empty.
func TestProfile_Validate_RoundCapsDefaultChain(t *testing.T) {
	tests := []struct {
		name    string
		profile []int
		cfg     []int
		want    []int
	}{
		{
			name:    "profile wins over config and built-in",
			profile: []int{2, 4},
			cfg:     []int{5, 8, 10},
			want:    []int{2, 4},
		},
		{
			name:    "config wins over built-in when profile is empty",
			profile: nil,
			cfg:     []int{3, 6},
			want:    []int{3, 6},
		},
		{
			name:    "built-in default when both are empty",
			profile: nil,
			cfg:     nil,
			want:    []int{5, 8, 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newValidProfile()
			p.RoundCaps = tt.profile
			cfg := Config{RoundCaps: tt.cfg}
			if err := p.validate(cfg); err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
			if !intSlicesEqual(p.RoundCaps, tt.want) {
				t.Errorf("resolved RoundCaps = %v; want %v", p.RoundCaps, tt.want)
			}
		})
	}
}

// TestProfile_Validate_JudgeModelDefaultChain exercises the three-level
// default resolution chain (profile > Config > built-in) for JudgeModel.
func TestProfile_Validate_JudgeModelDefaultChain(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		cfg     string
		want    string
	}{
		{
			name:    "profile wins over config and built-in",
			profile: "opus",
			cfg:     "sonnet",
			want:    "opus",
		},
		{
			name:    "config wins over built-in when profile is empty",
			profile: "",
			cfg:     "sonnet",
			want:    "sonnet",
		},
		{
			name:    "built-in default when both are empty",
			profile: "",
			cfg:     "",
			want:    "haiku",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newValidProfile()
			p.JudgeModel = tt.profile
			cfg := Config{JudgeModel: tt.cfg}
			if err := p.validate(cfg); err != nil {
				t.Fatalf("validate() = %v; want nil", err)
			}
			if p.JudgeModel != tt.want {
				t.Errorf("resolved JudgeModel = %q; want %q", p.JudgeModel, tt.want)
			}
		})
	}
}

// intSlicesEqual reports whether a and b contain the same ints in the same
// order.
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
