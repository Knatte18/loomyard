// transport.go implements authRT, the client's outermost http.RoundTripper and the sole owner of
// the Authorization header.
// It sets the header on every outbound request and, on a 401 response, invalidates the token cache,
// re-resolves once, and replays the request exactly once.

package githubclient

import (
	"fmt"
	"net/http"
)

// authRT owns the Authorization header on every request, resolving a token per
// request and replaying once with a fresh token on a 401 response.
type authRT struct {
	// transport is the underlying transport that performs the round trip.
	// A nil value defaults to http.DefaultTransport; tests inject an
	// alternative to point at an httptest server without a real network hop.
	transport http.RoundTripper
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
