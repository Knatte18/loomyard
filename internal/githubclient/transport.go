// transport.go implements authRT, the client's outermost http.RoundTripper and the sole owner of
// the Authorization header.
// It sets the header on every outbound request and, on a 401 response, invalidates the token cache,
// re-resolves once, and replays the request exactly once.

package githubclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// authRT owns the Authorization header on every request, resolving a token per
// request and replaying once with a fresh token on a 401 response.
type authRT struct {
	// transport is the underlying transport that performs the round trip.
	// A nil value defaults to http.DefaultTransport; tests inject an
	// alternative to point at an httptest server without a real network hop.
	transport http.RoundTripper
	// timeout bounds the original attempt and a 401 replay together, from the end of the pause hold.
	// Zero means no timeout.
	timeout time.Duration
}

// base returns the underlying transport, defaulting to http.DefaultTransport.
func (rt *authRT) base() http.RoundTripper {
	if rt.transport != nil {
		return rt.transport
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper.
// It sends req with a resolved token,
// and on a 401, it invalidates the cache and replays exactly once with a fresh token (unless the
// token is environment-sourced, in which case it returns an error instead).
//
// It first waits while a pause covers the process; the hold is outside the replay, so a replay is not held twice.
// The request then goes out under a context from requestContext, which gives a held call its full deadline after the hold.
func (rt *authRT) RoundTrip(req *http.Request) (*http.Response, error) {
	held, done := pauserecord.Network("github")
	defer done()

	ctx, cancel, err := rt.requestContext(req.Context(), held)
	if err != nil {
		return nil, err
	}
	resp, err := rt.authenticate(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// requestContext derives the context a request goes out under, once a hold of held has ended.
// It returns the caller's error at once when the caller's context was cancelled.
// For held zero the caller's context is used unchanged.
// For held above zero the caller's values are kept, its deadline, if any, moves later by held, and a cancellation of the caller before its own deadline passed still ends the call;
// once that deadline has passed, only the moved deadline does.
// A positive rt.timeout then applies from now.
// The returned cancel must run once the response body is closed, or at once on an error.
func (rt *authRT) requestContext(caller context.Context, held time.Duration) (context.Context, context.CancelFunc, error) {
	if err := caller.Err(); errors.Is(err, context.Canceled) {
		return nil, nil, err
	}
	ctx, cancel := caller, context.CancelFunc(func() {})
	if held > 0 {
		var cancelHeld context.CancelFunc
		if deadline, ok := caller.Deadline(); ok {
			ctx, cancelHeld = context.WithDeadline(context.WithoutCancel(caller), deadline.Add(held))
		} else {
			ctx, cancelHeld = context.WithCancel(context.WithoutCancel(caller))
		}
		stop := context.AfterFunc(caller, func() {
			if errors.Is(caller.Err(), context.Canceled) {
				cancelHeld()
			}
		})
		cancel = func() {
			stop()
			cancelHeld()
		}
	}
	if rt.timeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, rt.timeout)
		cancelOuter := cancel
		cancel = func() {
			cancelTimeout()
			cancelOuter()
		}
	}
	return ctx, cancel, nil
}

// cancelOnClose closes the response body it wraps and then cancels the context the request went out under.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

// Close closes the wrapped body, then cancels.
func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// authenticate sends req with a resolved token and handles the 401 replay.
func (rt *authRT) authenticate(req *http.Request) (*http.Response, error) {
	tok, source, err := resolveToken()
	if err != nil {
		return nil, err
	}

	resp, err := rt.send(req, tok)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	if source.isEnvSource() {
		// RoundTrip's contract (net/http.RoundTripper) forbids returning a
		// non-nil error alongside a response it obtained: "RoundTrip must
		// return err == nil if it obtained a response... A non-nil err
		// should be reserved for failure to obtain a response." Every real
		// caller reaches this transport through an http.Client, which
		// discards the response whenever RoundTrip returns a non-nil error
		// anyway, so closing the body and returning (nil, err) is
		// behavior-preserving while staying contract-compliant.
		resp.Body.Close()
		return nil, fmt.Errorf("githubclient: GitHub rejected the token from %s (401)", source.envName())
	}

	// The rejected token was cache- or gh-CLI-sourced: invalidate it so the
	// re-resolution below cannot simply read the same stale value back out
	// of the cache, then resolve and replay exactly once.
	invalidateCachedToken()
	resp.Body.Close()

	newTok, _, err := resolveToken()
	if err != nil {
		return nil, err
	}

	return rt.send(req, newTok)
}

// send clones req, sets its Authorization header to tok, rewinds the body
// via GetBody, and issues the clone through the underlying transport.
func (rt *authRT) send(req *http.Request, tok string) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+tok)

	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("githubclient: rewind request body: %w", err)
		}
		clone.Body = body
	}

	return rt.base().RoundTrip(clone)
}
