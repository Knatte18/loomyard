package battencli

import (
	"errors"
	"strings"
	"testing"
)

func TestRefuseNonPrime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		worktreeName string
		primeName    string
		primeNameErr error
		wantErr      bool
		wantSubstrs  []string
	}{
		{
			name:         "PrimeNameErrRefusesRegardlessOfNames",
			worktreeName: "some-worktree",
			primeName:    "some-worktree",
			primeNameErr: errors.New("no main worktree found"),
			wantErr:      true,
		},
		{
			name:         "MatchingNamesWithNoErrorSucceeds",
			worktreeName: "hub-repo",
			primeName:    "hub-repo",
			primeNameErr: nil,
			wantErr:      false,
		},
		{
			name:         "DifferingNamesRefuses",
			worktreeName: "task-slug",
			primeName:    "hub-repo",
			primeNameErr: nil,
			wantErr:      true,
			wantSubstrs:  []string{"task-slug", "hub-repo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := refuseNonPrime(tt.worktreeName, tt.primeName, tt.primeNameErr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("refuseNonPrime(%q, %q, %v) error = %v; wantErr %v", tt.worktreeName, tt.primeName, tt.primeNameErr, err, tt.wantErr)
			}
			if err == nil {
				return
			}
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refuseNonPrime(%q, %q, %v) = %q; want it to name %q", tt.worktreeName, tt.primeName, tt.primeNameErr, err.Error(), want)
				}
			}
		})
	}

	// The two refusal causes must read differently to the operator.
	fromPrimeNameErr := refuseNonPrime(tests[0].worktreeName, tests[0].primeName, tests[0].primeNameErr)
	fromNameMismatch := refuseNonPrime(tests[2].worktreeName, tests[2].primeName, tests[2].primeNameErr)
	if fromPrimeNameErr.Error() == fromNameMismatch.Error() {
		t.Errorf("the prime-name-error and name-mismatch refusal messages must be distinguishable; both read %q", fromNameMismatch.Error())
	}
}
