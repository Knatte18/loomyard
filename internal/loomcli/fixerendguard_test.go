// fixerendguard_test.go covers the goto guard that ends a pair's gate fixers once a move is admitted.

package loomcli

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// endCall is one recorded call of an end seam.
type endCall struct {
	dir, owner, role string
	callerPID        int
}

// TestEndGateFixers covers which processes a pair's fixer end reaches: each fixer role under the owner with the caller's pid, nothing for an empty slug, and the errors of the slug read and of an end.
func TestEndGateFixers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	slugOf := func(slug string, err error) func() (string, error) {
		return func() (string, error) { return slug, err }
	}
	failFirst := errors.New("end refused")
	tests := []struct {
		name      string
		slug      func() (string, error)
		endErr    error
		wantCalls []endCall
		wantErr   string
	}{
		{
			name: "ends every fixer role under the owner",
			slug: slugOf("pair", nil),
			wantCalls: []endCall{
				{"run-procs", "pair", gatefixer.Roles()[0], os.Getpid()},
				{"run-procs", "pair", gatefixer.Roles()[1], os.Getpid()},
			},
		},
		{name: "an empty slug ends nothing", slug: slugOf("", nil)},
		{name: "a slug read error is returned and ends nothing", slug: slugOf("", errors.New("origin unreadable")), wantErr: "origin unreadable"},
		{
			name:      "an error from the first role stops the loop and names the role",
			slug:      slugOf("pair", nil),
			endErr:    failFirst,
			wantCalls: []endCall{{"run-procs", "pair", gatefixer.Roles()[0], os.Getpid()}},
			wantErr:   gatefixer.Roles()[0],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []endCall
			end := func(dir, owner, role string, callerPID int) ([]runproc.Ended, error) {
				calls = append(calls, endCall{dir, owner, role, callerPID})
				return nil, tt.endErr
			}

			err := endGateFixers("run-procs", tt.slug, end)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("endGateFixers() = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("endGateFixers() = %v; want an error naming %q", err, tt.wantErr)
			}
			if tt.endErr != nil && !errors.Is(err, tt.endErr) {
				t.Errorf("endGateFixers() = %v; want it to wrap %v", err, tt.endErr)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("end calls = %v; want %v", calls, tt.wantCalls)
			}
		})
	}
}

// TestFixerEndingGuard covers when the wrapped goto guard ends the fixers: only once the inner guard admits, with the fixers' end error refusing the move, and a receiver with no location ending nothing.
func TestFixerEndingGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	endFailure := errors.New("fixer survived")
	innerFailure := errors.New("cannot decide")
	tests := []struct {
		name        string
		refusal     string
		innerErr    error
		endErr      error
		wantRefusal string
		wantErr     error
		wantEnds    int
	}{
		{name: "an admitting inner ends the fixers once and admits", wantEnds: 1},
		{name: "a refusing inner is returned and ends nothing", refusal: "settled", wantRefusal: "settled"},
		{name: "an inner error is returned and ends nothing", innerErr: innerFailure, wantErr: innerFailure},
		{name: "an end error is the guard's error", endErr: endFailure, wantErr: endFailure, wantEnds: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ends := 0
			guard := fixerEndingGuard(
				func(string) (string, error) { return tt.refusal, tt.innerErr },
				func() error { ends++; return tt.endErr },
			)

			refusal, err := guard("Plan-Write")

			if refusal != tt.wantRefusal || !errors.Is(err, tt.wantErr) {
				t.Errorf("guard() = %q, %v; want %q, %v", refusal, err, tt.wantRefusal, tt.wantErr)
			}
			if ends != tt.wantEnds {
				t.Errorf("fixers ended %d times; want %d", ends, tt.wantEnds)
			}
		})
	}

	t.Run("the guard specFor builds ends the fixers once the goto is admitted", func(t *testing.T) {
		t.Parallel()
		ends := 0
		c := &loomCLI{
			location:  &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."},
			runID:     "self",
			endFixers: func() error { ends++; return nil },
		}

		refusal, err := c.specFor("step").GotoGuard("Plan-Write")

		if refusal != "" || err != nil || ends != 1 {
			t.Errorf("GotoGuard() = %q, %v with %d ends; want an admission that ended the fixers once", refusal, err, ends)
		}
	})

	t.Run("a receiver with no location ends nothing", func(t *testing.T) {
		t.Parallel()
		c := &loomCLI{}
		guard := fixerEndingGuard(func(string) (string, error) { return "", nil }, c.endRunFixers)

		if refusal, err := guard("Plan-Write"); refusal != "" || err != nil {
			t.Errorf("guard() = %q, %v; want an admission", refusal, err)
		}
	})
}
