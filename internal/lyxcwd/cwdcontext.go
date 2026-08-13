// cwdcontext.go implements the context-carried per-call cwd-injection seam: WithCwd seeds an
// absolute directory into a context.Context, and CwdFrom reads it back, falling to Getwd (this
// package's sole os.Getwd wrapper) when the context carries none.
// This is what lets a caller thread an explicit cwd through a call chain instead of every callee
// reading the process working directory directly.

package lyxcwd

import (
	"context"
	"fmt"
	"path/filepath"
)

// cwdKey is the unexported key type WithCwd stores its value under, so no
// other package can collide with it by using the same string key.
type cwdKey struct{}

// WithCwd returns a copy of ctx carrying dir as the injected cwd that CwdFrom
// reads back.
// dir must be an absolute path;
// WithCwd panics otherwise, since a relative seed would resolve against
// whatever the process cwd happens to be — silently reintroducing the
// dependency this seam exists to remove.
// WithCwd never normalizes dir via filepath.Abs and never returns an error.
func WithCwd(ctx context.Context, dir string) context.Context {
	if dir == "" {
		panic(fmt.Sprintf("lyxcwd.WithCwd: dir must be an absolute path, got %q", dir))
	}
	if !filepath.IsAbs(dir) {
		panic(fmt.Sprintf("lyxcwd.WithCwd: dir must be an absolute path, got %q", dir))
	}
	return context.WithValue(ctx, cwdKey{}, dir)
}

// CwdFrom returns the cwd injected into ctx by WithCwd.
// When ctx carries no injected value, it falls back to Getwd, returning that
// call's error unchanged;
// this is the one place that fallback happens.
func CwdFrom(ctx context.Context) (string, error) {
	if dir, ok := ctx.Value(cwdKey{}).(string); ok {
		return dir, nil
	}
	return Getwd()
}
