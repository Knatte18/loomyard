// probe.go implements the one readiness gate every EnsureServer strategy
// runs regardless of how it got its connection — a fresh spawn for native,
// or a reused dial or a fresh spawn for supervised — before handing that
// connection back to the caller. Per the design's caution, even a
// -remote=auto/dialed connection can silently hand back a reference to a
// hung shared instance, so a successful initialize handshake alone is not
// sufficient proof of health; probe is what actually exercises the
// connection end-to-end.

package codeintelengine

import (
	"context"
	"time"
)

// probe issues an empty workspace/symbol query against client, bounded by
// its own timeout deadline, and returns whatever error that call produces
// verbatim: an ErrServerTimeout on deadline expiry, an lspError-derived
// wrap on a real protocol error, or nil on success. The empty-query result
// value itself is discarded — only the error matters, since probe's sole
// job is to prove the connection is alive and answering, not to resolve
// anything.
func probe(ctx context.Context, client *lspClient, timeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := client.workspaceSymbol(probeCtx, "")
	return err
}
