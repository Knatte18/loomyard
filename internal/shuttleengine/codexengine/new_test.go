// new_test.go holds New, the setup helper that builds a Codex engine with no dirs for the tests of its reading uses.

package codexengine

import "github.com/Knatte18/loomyard/internal/logger"

// New returns a Codex engine with the default redraw settle and no dirs, so it serves reading uses only.
func New() *Codex {
	return &Codex{submitRedrawSettleMS: defaultSubmitRedrawSettleMS, warn: logger.Warn, sandboxProbe: runSandboxProbe}
}
