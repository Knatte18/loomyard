package shedadapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEntryErr_HealthyContext(t *testing.T) {
	ctx := context.Background()
	if err := entryErr(ctx, "loom", "shuttle"); err != nil {
		t.Errorf("entryErr(healthy) = %v; want nil", err)
	}
}

func TestCancelErr_HealthyContext(t *testing.T) {
	ctx := context.Background()
	if err := cancelErr(ctx, "loom", "shuttle"); err != nil {
		t.Errorf("cancelErr(healthy) = %v; want nil", err)
	}
}

func TestEntryErr_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := entryErr(ctx, "loom", "shuttle")
	if err == nil {
		t.Fatal("entryErr(cancelled) = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "loom") {
		t.Errorf("entryErr error %q does not contain producer name %q", err.Error(), "loom")
	}
	if !strings.Contains(err.Error(), "shuttle") {
		t.Errorf("entryErr error %q does not contain engine label %q", err.Error(), "shuttle")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("entryErr(cancelled) = %v; want errors.Is(err, context.Canceled)", err)
	}
}

func TestCancelErr_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cancelErr(ctx, "loom", "shuttle")
	if err == nil {
		t.Fatal("cancelErr(cancelled) = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "loom") {
		t.Errorf("cancelErr error %q does not contain producer name %q", err.Error(), "loom")
	}
	if !strings.Contains(err.Error(), "shuttle") {
		t.Errorf("cancelErr error %q does not contain engine label %q", err.Error(), "shuttle")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelErr(cancelled) = %v; want errors.Is(err, context.Canceled)", err)
	}
}

func TestEntryErr_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond)

	err := entryErr(ctx, "loom", "shuttle")
	if err == nil {
		t.Fatal("entryErr(deadline exceeded) = nil; want non-nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("entryErr(deadline exceeded) = %v; want errors.Is(err, context.DeadlineExceeded)", err)
	}
}

func TestCancelErr_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond)

	err := cancelErr(ctx, "loom", "shuttle")
	if err == nil {
		t.Fatal("cancelErr(deadline exceeded) = nil; want non-nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancelErr(deadline exceeded) = %v; want errors.Is(err, context.DeadlineExceeded)", err)
	}
}

func TestEntryErrAndCancelErr_MessagesDiffer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entry := entryErr(ctx, "loom", "shuttle")
	exit := cancelErr(ctx, "loom", "shuttle")
	if entry.Error() == exit.Error() {
		t.Errorf("entryErr and cancelErr produced identical messages %q; want distinguishable text", entry.Error())
	}
}
