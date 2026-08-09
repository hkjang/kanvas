package auth

import (
	"net/http/httptest"
	"testing"
)

func TestRequestBaseURLHonorsForwardedHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "http://internal/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "wiki.example.test")
	if got := requestBaseURL(r); got != "https://wiki.example.test" {
		t.Fatalf("got %q", got)
	}
}
