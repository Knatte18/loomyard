// transient.go classifies a failed GitHub API call as transient:
// the call failed for a reason a retry a moment later is likely to clear.

package githubclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/url"

	"github.com/google/go-github/v75/github"
)

// IsTransient reports whether err's chain holds a GitHub API failure that a retry a moment later is likely to clear:
// a 5xx response, a transport-level *url.Error other than a cancellation, or context.DeadlineExceeded.
// Rate-limit errors are not transient, since an immediate retry hits the same limit;
// nor are context.Canceled, any 4xx response, or an error with none of these in its chain.
// A *url.Error from parsing a request URL or from a TLS certificate failure is not transport-level, so it is not transient either.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	var rateLimit *github.RateLimitError
	var abuse *github.AbuseRateLimitError
	if errors.As(err, &rateLimit) || errors.As(err, &abuse) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var resp *github.ErrorResponse
	if errors.As(err, &resp) {
		return resp.Response != nil && resp.Response.StatusCode >= http.StatusInternalServerError
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return false
	}
	return urlErr.Op != "parse" && !isCertificateFailure(urlErr)
}

// isCertificateFailure reports whether err's chain holds a TLS certificate verification failure.
// Such a failure is a persistent misconfiguration, never cleared by a retry.
func isCertificateFailure(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verification *tls.CertificateVerificationError
	return errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid) || errors.As(err, &verification)
}
