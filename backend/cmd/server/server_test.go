package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- AC-5: HTTPS redirect correctness ---

func TestBuildRedirectHandler_SimplePort(t *testing.T) {
	h := buildRedirectHandler(":8443")

	req := httptest.NewRequest(http.MethodGet, "/some/path?q=1", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	want := "https://example.com:8443/some/path?q=1"
	if loc != want {
		t.Errorf("expected redirect to %q, got %q", want, loc)
	}
}

func TestBuildRedirectHandler_HostWithPort(t *testing.T) {
	// When the incoming Host header already has a port, it should be stripped
	// before prepending the HTTPS port.
	h := buildRedirectHandler(":8443")

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.Host = "example.com:8080"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	loc := w.Header().Get("Location")
	want := "https://example.com:8443/page"
	if loc != want {
		t.Errorf("expected %q, got %q", want, loc)
	}
}

func TestBuildRedirectHandler_BindAllAddr(t *testing.T) {
	// LISTEN_ADDR="0.0.0.0:8443" — httpsPort should be just ":8443"
	addr := "0.0.0.0:8443"
	httpsPort := addr
	if colonIdx := strings.LastIndex(addr, ":"); colonIdx != -1 {
		httpsPort = addr[colonIdx:]
	}

	h := buildRedirectHandler(httpsPort)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.local"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	loc := w.Header().Get("Location")
	want := "https://myapp.local:8443/"
	if loc != want {
		t.Errorf("expected %q, got %q", want, loc)
	}
}

func TestBuildRedirectHandler_IPv4BindAddr(t *testing.T) {
	// LISTEN_ADDR="192.168.1.5:8443"
	addr := "192.168.1.5:8443"
	httpsPort := addr
	if colonIdx := strings.LastIndex(addr, ":"); colonIdx != -1 {
		httpsPort = addr[colonIdx:]
	}

	h := buildRedirectHandler(httpsPort)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Host = "myapp.internal"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	loc := w.Header().Get("Location")
	want := "https://myapp.internal:8443/api/test"
	if loc != want {
		t.Errorf("expected %q, got %q", want, loc)
	}
}

// --- AC-5: HSTS header ---

func TestBuildHSTSHandler_SetsHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := buildHSTSHandler(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("expected Strict-Transport-Security header to be set")
	}
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("expected max-age=31536000 in HSTS header, got %q", hsts)
	}
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("expected includeSubDomains in HSTS header, got %q", hsts)
	}
}

func TestBuildHSTSHandler_DelegatesResponse(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hello"))
	})
	h := buildHSTSHandler(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusTeapot {
		t.Errorf("expected inner handler status %d, got %d", http.StatusTeapot, w.Code)
	}
	if w.Body.String() != "hello" {
		t.Errorf("expected body 'hello', got %q", w.Body.String())
	}
}

// --- deriveMasterKey ---

func TestDeriveMasterKey_NonEmptySecret(t *testing.T) {
	k := deriveMasterKey("my-strong-secret")
	if len(k) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(k))
	}
}

func TestDeriveMasterKey_DifferentSecretsDifferentKeys(t *testing.T) {
	k1 := deriveMasterKey("secret-a")
	k2 := deriveMasterKey("secret-b")
	if string(k1) == string(k2) {
		t.Error("different secrets must produce different master keys")
	}
}

func TestDeriveMasterKey_SameSecretSameKey(t *testing.T) {
	k1 := deriveMasterKey("consistent-secret")
	k2 := deriveMasterKey("consistent-secret")
	if string(k1) != string(k2) {
		t.Error("same secret must always produce the same master key")
	}
}
