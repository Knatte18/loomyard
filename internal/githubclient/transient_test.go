package githubclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-github/v75/github"
)

func errResponse(status int) *github.ErrorResponse {
	return &github.ErrorResponse{Response: &http.Response{StatusCode: status}}
}

func TestIsTransient(t *testing.T) {
	dial := &url.Error{Op: "Get", URL: "https://api.github.com", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	rate := &github.RateLimitError{Response: &http.Response{StatusCode: http.StatusForbidden}}
	abuse := &github.AbuseRateLimitError{Response: &http.Response{StatusCode: http.StatusForbidden}}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"502", errResponse(502), true},
		{"503", errResponse(503), true},
		{"dial url error", dial, true},
		{"bare deadline", context.DeadlineExceeded, true},
		{"url error wrapping deadline", &url.Error{Op: "Get", Err: context.DeadlineExceeded}, true},
		{"wrapped 502", fmt.Errorf("x: %w", errResponse(502)), true},
		{"wrapped dial", fmt.Errorf("x: %w", dial), true},
		{"wrapped deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), true},
		{"wrapped url deadline", fmt.Errorf("x: %w", &url.Error{Err: context.DeadlineExceeded}), true},
		{"404", errResponse(404), false},
		{"422", errResponse(422), false},
		{"403", errResponse(403), false},
		{"rate limit", rate, false},
		{"wrapped rate limit", fmt.Errorf("x: %w", rate), false},
		{"abuse rate limit", abuse, false},
		{"bare canceled", context.Canceled, false},
		{"url error wrapping canceled", &url.Error{Op: "Get", Err: context.Canceled}, false},
		{"plain", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransient(tt.err); got != tt.want {
				t.Errorf("IsTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
