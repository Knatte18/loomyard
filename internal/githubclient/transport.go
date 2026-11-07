// transport.go implements authRT, the client's outermost http.RoundTripper.
// authRT is the sole owner of the Authorization header.
// On every outbound request it sets that header.
// On a 401 response it invalidates the token cache, resolves a token once more, and replays the request once.

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

// authRT owns the Authorization header on every request.
// It resolves a token per request and replays once with a fresh token on a 401 response.
type authRT struct {
	// transport is the underlying transport that performs the round trip.
	// A nil value defaults to http.DefaultTransport.
	// Tests set it to reach an httptest server without a real network hop.
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
// It sends req with a resolved token.
// On a 401 it invalidates the cache and replays once with a fresh token;
// an environment-sourced token instead yields an error.
//
// It first waits while a pause covers the process.
// The hold sits outside the replay, so a replay is not held twice.
// The request then goes out under a context that gives a held call its full deadline after the hold.
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
// For held above zero the caller's values are kept and its deadline, if any, moves later by held.
// A cancellation of the caller before its own deadline has passed still ends the call;
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
		// net/http's RoundTripper contract requires err == nil whenever a response was obtained.
		// An http.Client discards the response on any error, so closing the body first changes nothing for callers.
		resp.Body.Close()
		return nil, fmt.Errorf("%w: the token from %s (401)", ErrTokenRejected, source.envName())
	}

	// Invalidate the rejected cache or gh-CLI token, so the re-resolution cannot read it back.
	// Then resolve a token and replay the request once.
	invalidateCachedToken()
	resp.Body.Close()

	newTok, _, err := resolveToken()
	if err != nil {
		return nil, err
	}

	return rt.send(req, newTok)
}

// send clones req and sets its Authorization header to tok.
// It rewinds the body via GetBody and issues the clone through the underlying transport.
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
