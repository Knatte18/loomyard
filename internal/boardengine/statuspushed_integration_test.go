// statuspushed_integration_test.go covers Board.SetStatusPushed against a real board repository and a bare origin.
// It reuses the owner write's fixture, whose Bolt checkout sits beside the hub's records repository.

package boardengine

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBoard_SetStatusPushed asserts the status is on origin once the call returns.
// The detached sync, a no-op in a test binary, never makes that true.
// An unreachable push keeps the commit local and returns ErrOwnerOriginUnreachable.
// The skip variables commit without pushing.
//
// It is not parallel because the skip row sets the environment; the other rows say so themselves.
func TestBoard_SetStatusPushed(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "alpha"
	done := StatusDone

	openBoard := func(t *testing.T, f *ownerFixture, backoffSleep func(time.Duration)) *Board {
		t.Helper()
		cfg, err := LoadConfig(configengine.Dirs{Board: f.boardDir}, "board")
		if err != nil {
			t.Fatalf("load the board config: %v", err)
		}
		cfg.Path = f.boardDir
		cfg.BackoffSleep = backoffSleep
		return New(ApplySkipEnv(cfg))
	}
	statusOf := func(t *testing.T, tasks []Task) string {
		t.Helper()
		for _, task := range tasks {
			if task.Slug == slug {
				if task.Status == nil {
					return ""
				}
				return *task.Status
			}
		}
		t.Fatalf("no entry %q among %d entries", slug, len(tasks))
		return ""
	}
	originStatus := func(t *testing.T, f *ownerFixture) string {
		t.Helper()
		tasks, err := ParseBoardTasks([]byte(gitkit.Git(t, f.bare, "show", "main:board.json")))
		if err != nil {
			t.Fatalf("parse origin board.json: %v", err)
		}
		return statusOf(t, tasks)
	}
	localStatus := func(t *testing.T, board *Board) string {
		t.Helper()
		task, found, err := board.GetTask(slug)
		if err != nil || !found {
			t.Fatalf("GetTask(%q) = found %v, error %v; want the entry", slug, found, err)
		}
		if task.Status == nil {
			return ""
		}
		return *task.Status
	}

	rows := []struct {
		name string
		// setup runs after the fixture is built and before the write.
		setup   func(t *testing.T, f *ownerFixture)
		wantErr error
		// wantOriginStatus is the status origin holds afterwards.
		wantOriginStatus string
		// wantBackoff is the delays the push waits out between its attempts.
		wantBackoff []time.Duration
	}{
		{
			name:             "PushesBeforeItReturns",
			setup:            func(*testing.T, *ownerFixture) {},
			wantOriginStatus: StatusDone,
		},
		{
			name: "UnreachablePushKeepsTheCommitLocal",
			setup: func(t *testing.T, f *ownerFixture) {
				gitkit.Git(t, f.boardDir, "remote", "set-url", "--push", "origin", unreachableOrigin)
			},
			wantErr:     ErrOwnerOriginUnreachable,
			wantBackoff: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second},
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOwnerFixture(t, "neve")
			tt.setup(t, f)
			var delays []time.Duration
			board := openBoard(t, f, func(delay time.Duration) { delays = append(delays, delay) })

			err := board.SetStatusPushed(slug, &done)

			if !slices.Equal(delays, tt.wantBackoff) {
				t.Errorf("SetStatusPushed() backoff delays = %v; want %v", delays, tt.wantBackoff)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetStatusPushed() error = %v; want %v", err, tt.wantErr)
			}
			if got := originStatus(t, f); got != tt.wantOriginStatus {
				t.Errorf("origin status of %s = %q; want %q", slug, got, tt.wantOriginStatus)
			}
			if got := localStatus(t, board); got != StatusDone {
				t.Errorf("local status of %s = %q; want %q", slug, got, StatusDone)
			}
			if head := gitkit.RevParse(t, f.boardDir, "HEAD"); head == f.seedHead {
				t.Errorf("local head = %s; want the status commit on top of the seed", head)
			}
		})
	}

	t.Run("SkipPushCommitsOnly", func(t *testing.T) {
		t.Setenv("BOARD_SKIP_PUSH", "1")
		f := newOwnerFixture(t, "neve")
		board := openBoard(t, f, nil)

		if err := board.SetStatusPushed(slug, &done); err != nil {
			t.Fatalf("SetStatusPushed() error = %v; want nil", err)
		}

		if got := originStatus(t, f); got != "" {
			t.Errorf("origin status of %s = %q; want none, since BOARD_SKIP_PUSH pushes nothing", slug, got)
		}
		if got := localStatus(t, board); got != StatusDone {
			t.Errorf("local status of %s = %q; want %q", slug, got, StatusDone)
		}
		if head := gitkit.RevParse(t, f.boardDir, "HEAD"); head == f.seedHead {
			t.Errorf("local head = %s; want the status commit on top of the seed", head)
		}
	})
}
