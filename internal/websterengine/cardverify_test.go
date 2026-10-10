//go:build integration

// cardverify_test.go exercises rerunCardVerifies against real shell commands.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func verifyCard(n int, slug, command string) planparser.Card {
	return planparser.Card{Number: n, Slug: slug, Verify: command, HasVerify: command != ""}
}

func TestRerunCardVerifies_AllPass(t *testing.T) {
	cards := []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", "")}
	if got := rerunCardVerifies(cards, t.TempDir(), 0, nil, ""); len(got) != 0 {
		t.Fatalf("failures = %v, want none", got)
	}
}

func TestRerunCardVerifies_NonZeroExit(t *testing.T) {
	cards := []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", "exit 3")}
	got := rerunCardVerifies(cards, t.TempDir(), 0, nil, "")
	if len(got) != 1 || got[0] != "card 02-b verify exit 3 exited 3" {
		t.Fatalf("failures = %v", got)
	}
}

func TestRerunCardVerifies_Timeout(t *testing.T) {
	cards := []planparser.Card{verifyCard(4, "slow", "sleep 5")}
	got := rerunCardVerifies(cards, t.TempDir(), 200*time.Millisecond, nil, "")
	if len(got) != 1 || !strings.Contains(got[0], "timed out after 200ms") {
		t.Fatalf("failures = %v", got)
	}
}

func TestRerunCardVerifies_MissingDir(t *testing.T) {
	cards := []planparser.Card{verifyCard(5, "gone", "true")}
	got := rerunCardVerifies(cards, filepath.Join(t.TempDir(), "nope"), 0, nil, "")
	if len(got) != 1 || !strings.Contains(got[0], "card 05-gone verify true ") {
		t.Fatalf("failures = %v", got)
	}
}

func TestRerunCardVerifies_Slotted(t *testing.T) {
	t.Parallel()

	const requireCap = `case "$GOFLAGS" in *-p=6*) true;; *) exit 9;; esac`
	tests := []struct {
		name        string
		limits      func() (gateslot.Limits, error)
		wantFailure string
	}{
		{
			name:   "the command sees the pool's -p cap",
			limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1, GoParallel: 6}, nil },
		},
		{
			name:        "a failed acquire is a failing card line",
			limits:      func() (gateslot.Limits, error) { return gateslot.Limits{}, errors.New("gate.yaml unreadable") },
			wantFailure: "card 07-slotted verify " + requireCap + " could not acquire a gate slot: read gate limits: gate.yaml unreadable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := &gateslot.Pool{Dir: filepath.Join(t.TempDir(), "gate"), Limits: tc.limits, Poll: 10 * time.Millisecond}
			waitDir := filepath.Join(t.TempDir(), "wait")

			got := rerunCardVerifies([]planparser.Card{verifyCard(7, "slotted", requireCap)}, t.TempDir(), 0, pool, waitDir)

			switch {
			case tc.wantFailure == "" && len(got) != 0:
				t.Errorf("failures = %v; want none", got)
			case tc.wantFailure != "" && (len(got) != 1 || got[0] != tc.wantFailure):
				t.Errorf("failures = %v; want [%q]", got, tc.wantFailure)
			}
			if entries, err := os.ReadDir(waitDir); err != nil || len(entries) != 0 {
				t.Errorf("wait directory = (%v, %v); want it to hold no record once the rerun returns", entries, err)
			}
			if holders, err := pool.Holders(); err != nil || len(holders) != 0 {
				t.Errorf("Holders() = (%v, %v); want the slot released", holders, err)
			}
		})
	}
}
