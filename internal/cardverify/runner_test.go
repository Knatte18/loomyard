//go:build tier1

// runner_test.go exercises Runner against real shell lines and a temp gate-slot pool.

package cardverify

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func verifyCard(n int, slug, command string) planparser.Card {
	return planparser.Card{Number: n, Slug: slug, Verify: command, HasVerify: command != ""}
}

func newPool(t *testing.T, limits func() (gateslot.Limits, error)) *gateslot.Pool {
	t.Helper()
	return &gateslot.Pool{Dir: filepath.Join(t.TempDir(), "gate"), Limits: limits, Poll: 10 * time.Millisecond}
}

// TestRunner does not call t.Parallel: two subtests set the process environment through t.Setenv, which forbids it in the test and its ancestors.
// Those subtests run first, in order, and the parallel ones resume after.
func TestRunner(t *testing.T) {
	t.Run("a stale prebuilt-lyx variable never reaches a line", func(t *testing.T) {
		t.Setenv(gateslot.PrebuiltLyxEnv, "/stale/lyx")
		runner := Runner{Worktree: t.TempDir()}
		cards := []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", ""), verifyCard(3, "c", `test -z "$`+gateslot.PrebuiltLyxEnv+`"`)}
		if got := runner.RunCards(cards, io.Discard, false); len(got) != 0 {
			t.Fatalf("RunCards() = %v; want no failures", got)
		}
	})

	t.Run("a gate call takes no slot and no runner timeout bounds it", func(t *testing.T) {
		stubDir := t.TempDir()
		stub := "#!/bin/sh\nsleep 1\nexit 0\n"
		if err := os.WriteFile(filepath.Join(stubDir, "lyx"), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		pool := newPool(t, func() (gateslot.Limits, error) { return gateslot.Limits{}, errors.New("a gate call reads no limits") })
		runner := Runner{Worktree: t.TempDir(), Slots: pool, Timeout: 200 * time.Millisecond}

		if got := runner.RunLine("lyx gate test --wait ./x", io.Discard); got != "" {
			t.Errorf("RunLine(gate call) = %q; want a pass", got)
		}
		if got := runner.RunLine("lyx gate test ./x", io.Discard); !strings.Contains(got, "could not acquire a gate slot") {
			t.Errorf("RunLine(gate call without --wait) = %q; want it slot-held", got)
		}
	})

	t.Run("unslotted lines", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "nope")
		tests := []struct {
			name     string
			worktree string
			timeout  time.Duration
			cards    []planparser.Card
			stop     bool
			want     func(got []string) bool
			wantDesc string
		}{
			{
				name:     "a passing and a non-zero line give the record-batch failure line",
				cards:    []planparser.Card{verifyCard(1, "a", "true"), verifyCard(2, "b", "exit 3")},
				want:     func(got []string) bool { return slices.Equal(got, []string{"card 02-b verify exit 3 exited 3"}) },
				wantDesc: `["card 02-b verify exit 3 exited 3"]`,
			},
			{
				name:     "stopAtFirst ends the run at the first failing card",
				cards:    []planparser.Card{verifyCard(1, "a", "exit 1"), verifyCard(2, "b", "exit 2")},
				stop:     true,
				want:     func(got []string) bool { return slices.Equal(got, []string{"card 01-a verify exit 1 exited 1"}) },
				wantDesc: `["card 01-a verify exit 1 exited 1"]`,
			},
			{
				name:  "without stopAtFirst every failing card is named",
				cards: []planparser.Card{verifyCard(1, "a", "exit 1"), verifyCard(2, "b", "exit 2")},
				want: func(got []string) bool {
					return slices.Equal(got, []string{"card 01-a verify exit 1 exited 1", "card 02-b verify exit 2 exited 2"})
				},
				wantDesc: "both failing cards",
			},
			{
				name:     "a line past the timeout is named",
				timeout:  200 * time.Millisecond,
				cards:    []planparser.Card{verifyCard(4, "slow", "sleep 5")},
				want:     func(got []string) bool { return len(got) == 1 && strings.Contains(got[0], "timed out after 200ms") },
				wantDesc: "a timed out after 200ms line",
			},
			{
				name:     "a missing worktree fails the line",
				worktree: missing,
				cards:    []planparser.Card{verifyCard(5, "gone", "true")},
				want: func(got []string) bool {
					return len(got) == 1 && strings.HasPrefix(got[0], "card 05-gone verify true ")
				},
				wantDesc: "a failing card 05-gone line",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				worktree := tt.worktree
				if worktree == "" {
					worktree = t.TempDir()
				}
				runner := Runner{Worktree: worktree, Timeout: tt.timeout}
				if got := runner.RunCards(tt.cards, io.Discard, tt.stop); !tt.want(got) {
					t.Errorf("RunCards() = %v; want %s", got, tt.wantDesc)
				}
			})
		}
	})

	t.Run("slot-held lines", func(t *testing.T) {
		t.Parallel()

		const requireFlags = `case "$GOFLAGS" in *"-p=6 -trimpath"*) true;; *) exit 9;; esac`
		tests := []struct {
			name        string
			limits      func() (gateslot.Limits, error)
			command     string
			timeout     time.Duration
			wantFailure string
		}{
			{
				name:    "the line sees the pool's tier0 parallel and -trimpath through HolderEnv",
				limits:  func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1, Tier0Parallel: 6}, nil },
				command: requireFlags,
			},
			{
				name:        "a line past the timeout once its slot is held is named",
				limits:      func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1}, nil },
				command:     "sleep 5",
				timeout:     200 * time.Millisecond,
				wantFailure: "card 07-slotted verify sleep 5 timed out after 200ms",
			},
			{
				name:        "unusable limits are a failing line naming the cause",
				limits:      func() (gateslot.Limits, error) { return gateslot.Limits{}, errors.New("gate.yaml is unreadable") },
				command:     "true",
				wantFailure: "card 07-slotted verify true could not acquire a gate slot: read gate limits: gate.yaml is unreadable",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				pool := newPool(t, tt.limits)
				waitDir := filepath.Join(t.TempDir(), "wait")
				runner := Runner{Worktree: t.TempDir(), Slots: pool, WaitDir: waitDir, Timeout: tt.timeout}

				got := runner.RunCards([]planparser.Card{verifyCard(7, "slotted", tt.command)}, io.Discard, false)

				switch {
				case tt.wantFailure == "" && len(got) != 0:
					t.Errorf("RunCards() = %v; want no failures", got)
				case tt.wantFailure != "" && !slices.Equal(got, []string{tt.wantFailure}):
					t.Errorf("RunCards() = %v; want [%q]", got, tt.wantFailure)
				}
				if entries, err := os.ReadDir(waitDir); err != nil || len(entries) != 0 {
					t.Errorf("wait directory = (%v, %v); want it to hold no record once the run returns", entries, err)
				}
				if holders, err := pool.Holders(); err != nil || len(holders) != 0 {
					t.Errorf("Holders() = (%v, %v); want the slot released", holders, err)
				}
			})
		}
	})
}
