//go:build tier0

// skip_test.go pins which run lock holders pass the pre-end wait and the wording of the refusal for those that do not.

package pairteardown

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// TestRunLockHolderPasses pins the pure rule of the pre-end run-lock wait and, on each refusing row, the refusal's wording.
func TestRunLockHolderPasses(t *testing.T) {
	t.Parallel()

	const holder = 4242
	own := []pauserecord.WaitingMarker{{PID: holder, Slug: "slug"}}
	otherSlug := []pauserecord.WaitingMarker{{PID: holder, Slug: "other"}}
	otherPID := []pauserecord.WaitingMarker{{PID: 7, Slug: "slug"}}
	const retry = "the removal"
	tests := []struct {
		name         string
		holderKnown  bool
		waiting      []pauserecord.WaitingMarker
		paused       bool
		hubEntry     bool
		parentLocked bool
		goos         string
		wantPass     bool
		// wantContains and wantNotContains are substrings of the refusal.
		wantContains    []string
		wantNotContains []string
	}{
		{name: "MarkerNamingTheHoldersPidPasses", holderKnown: true, waiting: own, wantPass: true},
		{name: "MarkerNamingTheHoldersPidPassesUnderAParentLockRecord", holderKnown: true, waiting: own, parentLocked: true, wantPass: true},
		{name: "PausedWithNoParentLockRecordPasses", holderKnown: true, paused: true, wantPass: true},
		{name: "PausedWithNoParentLockRecordPassesAnUnknownHolder", paused: true, wantPass: true},
		{
			name: "MarkerOfAnotherSlugDoesNotPass", holderKnown: true, waiting: otherSlug,
			wantContains: []string{"pid 4242", "lym tender pause slug", "run the removal again"},
		},
		{
			name: "MarkerOfAnotherPidDoesNotPassUnpaused", holderKnown: true, waiting: otherPID,
			wantContains: []string{"pid 4242", "lym tender pause slug", "run the removal again once that pid ends"},
		},
		{
			name: "UnpausedHolderWithNoMarkerNamesThePauseThenTheRetry", holderKnown: true,
			wantContains: []string{"pid 4242", "lym tender pause slug", "run the removal again"},
		},
		{
			name: "UnpausedHolderUnderAParentLockRecordNamesNoPause", holderKnown: true, parentLocked: true,
			wantContains:    []string{"pid 4242", "run the removal again once that pid ends"},
			wantNotContains: []string{"lym tender pause"},
		},
		{
			name: "PausedHolderUnderAParentLockRecordNamesTheSlugResume", holderKnown: true, waiting: otherPID, paused: true, parentLocked: true,
			wantContains:    []string{"pid 4242", "lym tender resume slug", "held for another machine", "never lift that pause", "run the removal again"},
			wantNotContains: []string{"lifts every live entry"},
		},
		{
			name: "PausedUnknownHolderUnderAParentLockRecordNamesTheSlugResumeWithNoPid", paused: true, parentLocked: true,
			wantContains:    []string{"an unknown process", "lym tender resume slug", "never lift that pause"},
			wantNotContains: []string{"pid 4242"},
		},
		{
			name: "HubEntryResumeNamesNoSlugAndWhatItLifts", holderKnown: true, paused: true, hubEntry: true, parentLocked: true,
			wantContains:    []string{"pid 4242", "lym tender resume` with no slug", "lifts every live entry of the hub", "other pairs' pauses", "run the removal again"},
			wantNotContains: []string{"lym tender resume slug"},
		},
		{
			name: "HubEntryBesideASlugEntryResumeNamesNoSlug", paused: true, hubEntry: true, parentLocked: true,
			wantContains:    []string{"lym tender resume` with no slug", "lifts every live entry of the hub"},
			wantNotContains: []string{"lym tender resume slug"},
		},
		{
			name: "UnknownUnpausedHolderOnLinuxNamesTheLockAndTenderStatus", goos: "linux",
			wantContains: []string{"an unknown process", "lym tender pause slug", "run.lock", "lym tender status", "run the removal again"},
		},
		{
			name: "UnknownUnpausedHolderOnWindowsNamesTaskManager", goos: "windows",
			wantContains:    []string{"an unknown process", "lym tender pause slug", "run.lock", "Task Manager"},
			wantNotContains: []string{"lym tender status"},
		},
		{
			name: "UnknownUnpausedHolderUnderAParentLockRecordNamesNoPause", parentLocked: true, goos: "linux",
			wantContains:    []string{"an unknown process", "lym tender status"},
			wantNotContains: []string{"lym tender pause"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := runLockHolderPasses(holder, tt.holderKnown, tt.waiting, "slug", tt.paused, tt.parentLocked); got != tt.wantPass {
				t.Fatalf("runLockHolderPasses() = %v; want %v", got, tt.wantPass)
			}
			if tt.wantPass {
				return
			}
			err := runLockRefusal(holder, tt.holderKnown, "slug", "/scratch/run.lock", tt.goos, retry, tt.paused, tt.hubEntry, tt.parentLocked)
			if !errors.Is(err, ErrDriverBusy) {
				t.Fatalf("runLockRefusal() = %v; want one wrapping ErrDriverBusy", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q lacks %q", err.Error(), want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("refusal %q names %q", err.Error(), notWant)
				}
			}
		})
	}
}
