package lifecyclecli

import (
	"errors"
	"strings"
	"testing"
)

func TestRefuseNonPrime(t *testing.T) {
	tests := []struct {
		name         string
		worktreeName string
		primeName    string
		primeNameErr error
		wantErr      bool
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseNonPrime(tt.worktreeName, tt.primeName, tt.primeNameErr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("refuseNonPrime(%q, %q, %v) error = %v; wantErr %v", tt.worktreeName, tt.primeName, tt.primeNameErr, err, tt.wantErr)
			}
		})
	}
}

func TestRefuseNonPrime_MessagesAreDistinguishable(t *testing.T) {
	errFromPrimeNameErr := refuseNonPrime("some-worktree", "some-worktree", errors.New("no main worktree found"))
	errFromNameMismatch := refuseNonPrime("task-slug", "hub-repo", nil)

	if errFromPrimeNameErr == nil || errFromNameMismatch == nil {
		t.Fatalf("both refusals must be non-nil; got %v and %v", errFromPrimeNameErr, errFromNameMismatch)
	}
	if errFromPrimeNameErr.Error() == errFromNameMismatch.Error() {
		t.Errorf("the two refusal messages must be distinguishable; both read %q", errFromPrimeNameErr.Error())
	}
	if !strings.Contains(errFromNameMismatch.Error(), "task-slug") || !strings.Contains(errFromNameMismatch.Error(), "hub-repo") {
		t.Errorf("name-mismatch refusal = %q; want it to name both %q and %q", errFromNameMismatch.Error(), "task-slug", "hub-repo")
	}
}
