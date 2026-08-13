// cwdcontext_test.go exercises the context-carried cwd seam: a context seeded via WithCwd hands its
// value back through CwdFrom, a bare context falls back to Getwd, and WithCwd panics on an empty or
// relative dir.

package lyxcwd

import (
	"context"
	"testing"
)

// TestWithCwd_RoundTrip verifies that a context seeded via WithCwd returns
// that exact value from CwdFrom.
func TestWithCwd_RoundTrip(t *testing.T) {
	t.Parallel()

	const want = "/abs/path/to/worktree"
	ctx := WithCwd(context.Background(), want)

	got, err := CwdFrom(ctx)
	if err != nil {
		t.Fatalf("CwdFrom() error = %v; want nil", err)
	}
	if got != want {
		t.Errorf("CwdFrom() = %q; want %q", got, want)
	}
}

// TestCwdFrom_FallsBackToGetwd verifies that a bare context.Background()
// makes CwdFrom return the same answer as Getwd.
func TestCwdFrom_FallsBackToGetwd(t *testing.T) {
	t.Parallel()

	want, wantErr := Getwd()

	got, err := CwdFrom(context.Background())
	if wantErr != nil {
		if err == nil {
			t.Fatalf("CwdFrom() error = nil; want an error matching Getwd()'s %v", wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("CwdFrom() error = %v; want nil", err)
	}
	if got != want {
		t.Errorf("CwdFrom() = %q; want %q", got, want)
	}
}

// TestWithCwd_PanicsOnEmpty verifies that WithCwd panics when dir is empty.
func TestWithCwd_PanicsOnEmpty(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("WithCwd(ctx, \"\") did not panic; want a panic")
		}
	}()
	WithCwd(context.Background(), "")
}

// TestWithCwd_PanicsOnRelative verifies that WithCwd panics when dir is not
// an absolute path.
func TestWithCwd_PanicsOnRelative(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("WithCwd(ctx, \"relative/x\") did not panic; want a panic")
		}
	}()
	WithCwd(context.Background(), "relative/x")
}
