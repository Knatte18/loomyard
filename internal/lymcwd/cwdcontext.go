// cwdcontext.go carries an explicit working directory through a context.Context.
// WithCwd seeds an absolute directory into a context.
// CwdFrom reads it back, falling back to Getwd when the context carries none.
// Getwd is this package's sole wrapper around os.Getwd.
// Threading the cwd down a call chain keeps callees from reading the process working directory directly.

package lymcwd

import (
	"context"
	"fmt"
	"path/filepath"
)

// cwdKey is the unexported context key type under which the injected cwd is stored.
// Its unexported type keeps other packages from colliding with it, even when they use the same string key.
type cwdKey struct{}

// WithCwd returns a copy of ctx that carries dir as its injected working directory.
// It panics unless dir is absolute.
// A relative path would resolve against the process working directory, the dependency injection removes.
// It does not normalize dir with filepath.Abs and never returns an error.
func WithCwd(ctx context.Context, dir string) context.Context {
	if dir == "" {
		panic(fmt.Sprintf("lymcwd.WithCwd: dir must be an absolute path, got %q", dir))
	}
	if !filepath.IsAbs(dir) {
		panic(fmt.Sprintf("lymcwd.WithCwd: dir must be an absolute path, got %q", dir))
	}
	return context.WithValue(ctx, cwdKey{}, dir)
}

// CwdFrom returns the cwd injected into ctx.
// When ctx carries none, it falls back to Getwd and returns that call's error unchanged.
// This is the one place that fallback happens.
func CwdFrom(ctx context.Context) (string, error) {
	if dir, ok := ctx.Value(cwdKey{}).(string); ok {
		return dir, nil
	}
	return Getwd()
}
