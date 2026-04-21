package cleanxff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHandler(t *testing.T, cidrs []string) http.Handler {
	t.Helper()
	cfg := CreateConfig()
	cfg.TrustedCIDRs = cidrs

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	h, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return h
}

func runAndGetXFF(t *testing.T, h http.Handler, incoming string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	if incoming != "" {
		req.Header.Set("X-Forwarded-For", incoming)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return req.Header.Get("X-Forwarded-For")
}

func TestRemovesTrustedIP(t *testing.T) {
	h := newHandler(t, []string{"10.0.0.0/8", "173.245.48.0/20"})

	got := runAndGetXFF(t, h, "1.1.1.1, 173.245.48.5, 10.0.0.7")
	want := "1.1.1.1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKeepsUntrustedHops(t *testing.T) {
	h := newHandler(t, []string{"10.0.0.0/8"})

	got := runAndGetXFF(t, h, "1.1.1.1, 5.5.5.5, 10.0.0.7")
	want := "1.1.1.1, 5.5.5.5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemovesAllWhenAllTrusted(t *testing.T) {
	h := newHandler(t, []string{"10.0.0.0/8"})

	got := runAndGetXFF(t, h, "10.0.0.5, 10.0.0.7")
	if got != "" {
		t.Errorf("expected empty XFF, got %q", got)
	}
}

func TestEmptyXFFUnchanged(t *testing.T) {
	h := newHandler(t, []string{"10.0.0.0/8"})

	got := runAndGetXFF(t, h, "")
	if got != "" {
		t.Errorf("expected empty XFF, got %q", got)
	}
}

func TestIPv6Trusted(t *testing.T) {
	h := newHandler(t, []string{"2001:db8::/32"})

	got := runAndGetXFF(t, h, "2001:db8::1, 1.1.1.1")
	want := "1.1.1.1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMultipleHeaderValues(t *testing.T) {
	cfg := CreateConfig()
	cfg.TrustedCIDRs = []string{"10.0.0.0/8"}

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	h, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Add("X-Forwarded-For", "1.1.1.1, 10.0.0.7")
	req.Header.Add("X-Forwarded-For", "10.0.0.8, 2.2.2.2")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	got := req.Header.Get("X-Forwarded-For")
	want := "1.1.1.1, 2.2.2.2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvalidCIDR(t *testing.T) {
	cfg := CreateConfig()
	cfg.TrustedCIDRs = []string{"not-a-cidr"}

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	_, err := New(context.Background(), next, cfg, "test")
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestEmptyCIDRsFails(t *testing.T) {
	cfg := CreateConfig()
	cfg.TrustedCIDRs = nil

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	_, err := New(context.Background(), next, cfg, "test")
	if err == nil {
		t.Fatal("expected error for empty TrustedCIDRs, got nil")
	}
}

func TestNonIPTokenPreserved(t *testing.T) {
	h := newHandler(t, []string{"10.0.0.0/8"})

	// Paranoid case: XFF containing a non-IP token (shouldn't happen in
	// practice, but we don't want to silently drop it).
	got := runAndGetXFF(t, h, "1.1.1.1, unknown, 10.0.0.7")
	want := "1.1.1.1, unknown"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
