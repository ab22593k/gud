package request

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/genai"
	"google.golang.org/genai/interactions/models/apierrors"
)

func TestIsTransientErr_Classifies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"timeout-like", errors.New("model error: timeout exceeded"), true},
		{"unavailable", errors.New("model error: unavailable"), true},
		{"rate-limited", errors.New("model error code: 429"), true},
		{"server-error", errors.New("model error code: 500"), true},
		{"invalid-key", errors.New("model error code: 401"), false},
		{"forbidden", errors.New("model error code: 403"), false},
		{"empty", errors.New("generated message is empty"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isTransientErr(tc.err); got != tc.want {
				t.Errorf("isTransientErr(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsTransientErr_RealAPIErrorShape pins classification to the error text the
// Gemini client actually produces, not a hand-written approximation.
//
// isTransientErr classifies by substring match, and the client renders an APIError
// as "Error %d, Message: %s, Status: %s, Details: %v" — so the HTTP status code
// only reaches the matcher because that formatter interpolates it. If a future
// client release changes that format, retries stop firing silently and the commit
// tool gives up instead of retrying. Constructing a real genai.APIError makes this
// test fail if that happens.
func TestIsTransientErr_RealAPIErrorShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   int
		status string
		want   bool
	}{
		{"rate-limited", 429, "429 Too Many Requests", true},
		{"internal-server-error", 500, "500 Internal Server Error", true},
		{"bad-gateway", 502, "502 Bad Gateway", true},
		{"service-unavailable", 503, "503 Service Unavailable", true},
		{"gateway-timeout", 504, "504 Gateway Timeout", true},
		{"unauthorized", 401, "401 Unauthorized", false},
		{"forbidden", 403, "403 Forbidden", false},
		{"bad-request", 400, "400 Bad Request", false},
		{"not-found", 404, "404 Not Found", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Built the way the production call path surfaces it: the client's own
			// error, wrapped with the operation context that failed.
			apiErr := genai.APIError{Code: tc.code, Status: tc.status, Message: "upstream rejected the request"}
			wrapped := fmt.Errorf("model error: %w", apiErr)

			if got := isTransientErr(wrapped); got != tc.want {
				t.Errorf("isTransientErr(%v)=%v, want %v", wrapped, got, tc.want)
			}
		})
	}
}

// TestIsTransientErr_StatusCodeReachesTheMatcher is the direct assertion behind the
// test above: it pins the assumption the substring matcher depends on, so a client
// release that stops interpolating the status code fails here with an explanation
// rather than as a mysterious production retry regression.
func TestIsTransientErr_StatusCodeReachesTheMatcher(t *testing.T) {
	t.Parallel()

	apiErr := genai.APIError{Code: 429, Status: "429 Too Many Requests", Message: "quota exceeded"}
	if got := apiErr.Error(); !strings.Contains(got, "429") {
		t.Errorf("APIError.Error() = %q, expected it to carry the HTTP status code; "+
			"isTransientErr matches on substrings and would silently stop retrying", got)
	}
}

// TestIsTransientErr_InteractionsAPIErrorShape is the same lock for the error
// type the Interactions API actually returns.
//
// The Interactions client does not use genai.APIError: every 4xx and 5xx
// becomes an apierrors.APIError rendering "API error occurred: Status 429".
// The HTTP status still reaches the matcher, so retries still fire — but only
// because of that format string. A change to it would end retries silently,
// which is the exact failure this file exists to prevent.
func TestIsTransientErr_InteractionsAPIErrorShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code int
		want bool
	}{
		{"rate-limited", 429, true},
		{"internal-server-error", 500, true},
		{"bad-gateway", 502, true},
		{"service-unavailable", 503, true},
		{"gateway-timeout", 504, true},
		{"unauthorized", 401, false},
		{"forbidden", 403, false},
		{"bad-request", 400, false},
		{"not-found", 404, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			apiErr := apierrors.NewAPIError("API error occurred", tc.code, `{"error":{"message":"nope"}}`, nil)
			wrapped := fmt.Errorf("model error: %w", apiErr)

			if got := isTransientErr(wrapped); got != tc.want {
				t.Errorf("isTransientErr(%v)=%v, want %v", wrapped, got, tc.want)
			}
		})
	}
}

func TestIsTransientErr_InteractionsStatusCodeReachesTheMatcher(t *testing.T) {
	t.Parallel()

	apiErr := apierrors.NewAPIError("API error occurred", 429, `{"error":{"message":"quota"}}`, nil)
	if got := apiErr.Error(); !strings.Contains(got, "429") {
		t.Errorf("apierrors.APIError.Error() = %q, expected it to carry the HTTP status "+
			"code; isTransientErr matches on substrings and retries would stop firing", got)
	}
}
