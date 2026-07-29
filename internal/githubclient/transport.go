// transport.go implements authRT, the client's outermost http.RoundTripper
// and the sole owner of the Authorization header. It sets the header on
// every outbound request and, on a 401 response, invalidates the token
// cache, re-resolves once, and replays the request exactly once.

package githubclient

import (
	"fmt"
	"net/http"
)

// authRT is the sole owner of the Authorization header on every request the
// client sends. It resolves a token per request -- so a token refreshed by a
// prior 401 is picked up immediately on the next call -- and, on a 401
// response, invalidates the cache and replays exactly once with a freshly
// resolved token. Its own fields hold no mutable state; the token cache and
// resolveToken each serialize their own access, so authRT is safe for
// concurrent use.
type authRT struct {
	// transport is the underlying transport that performs the round trip.
	// A nil value defaults to http.DefaultTransport; tests inject an
	// alternative to point at an httptest server without a real network hop.
	transport http.RoundTripper
}

// base returns the underlying transport to delegate to, defaulting to
// http.DefaultTransport when none was configured.
func (rt *authRT) base() http.RoundTripper {
	if rt.transport != nil {
		return rt.transport
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper. It resolves a token, sends req
// with that token via send, and inspects the response. A non-401 response
// is returned unchanged. On a 401, an environment-sourced token (GH_TOKEN or
// GITHUB_TOKEN) is never invalidated and replayed -- environment variables
// outrank the cache by rule, so re-resolution is guaranteed to reproduce the
// identical value and the replay would be a guaranteed-identical second
// failure -- so the 401 response is discarded and an error naming the
// rejected environment variable is returned instead. Otherwise the cache is
// invalidated and the request is replayed exactly once with a freshly
// resolved token; a second consecutive 401 is returned to the caller
// unchanged, never a loop.
func (rt *authRT) RoundTrip(req *http.Request) (*http.Response, error) {
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
// via GetBody when present, and issues the clone through the underlying
// transport. Cloning is required by the http.RoundTripper contract, which
// forbids mutating the request it is handed -- here the same request object
// may be sent twice (the original attempt and the 401 replay), so mutating
// it in place would corrupt the caller's own copy. The GetBody rewind is
// load-bearing rather than incidental: issue creation is a POST with a JSON
// body, and skipping it would send probably-already-drained bytes on a
// replay, surfacing as a confusing GitHub validation error rather than as a
// missing rewind.
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
