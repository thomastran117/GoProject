package microsoft

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"backend/internal/application/middleware"

	jwt "github.com/golang-jwt/jwt/v5"
)

// urlOverrideTransport redirects all HTTP requests to a fixed base URL,
// preserving the original path and query string.
type urlOverrideTransport struct {
	serverURL string
}

func (t *urlOverrideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, _ := url.Parse(t.serverURL)
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = target.Scheme
	req2.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req2)
}

// assertAPIError fails the test unless err is a *middleware.APIError with the expected code.
func assertAPIError(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *middleware.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != code {
		t.Errorf("expected code %q, got %q", code, apiErr.Code)
	}
}

// --- helpers ---

// generateTestRSAKey returns a 2048-bit RSA private key and a base64-encoded
// DER certificate (suitable for use as an x5c value in a JWKS).
func generateTestRSAKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return key, base64.StdEncoding.EncodeToString(der)
}

// signToken creates a signed RS256 JWT with the given key, kid, and claims.
func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return s
}

// newJWKSServer starts a test server that returns a JWKS containing one key.
func newJWKSServer(t *testing.T, kid, x5c string) *httptest.Server {
	t.Helper()
	keys := jwkSet{Keys: []jwk{{Kid: kid, X5C: []string{x5c}}}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(keys)
	}))
}

// validClaims returns a minimal set of valid Microsoft ID token claims.
func validClaims() Claims {
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{"test-client-id"},
			Issuer:    "https://login.microsoftonline.com/test-tenant/v2.0",
		},
		OID:   "test-oid-123",
		Email: "user@example.com",
	}
}

// newTestClient builds a Client whose httpClient hits the given server.
func newTestClient(srv *httptest.Server, clientID string) *Client {
	return NewClient(&http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}, clientID)
}

// --- jwksCache ---

func TestJWKSCache_SetAndGet(t *testing.T) {
	var c jwksCache
	keys := &jwkSet{Keys: []jwk{{Kid: "k1"}}}
	c.set(keys)

	got := c.get()
	if got == nil {
		t.Fatal("expected cached keys, got nil")
	}
	if len(got.Keys) != 1 || got.Keys[0].Kid != "k1" {
		t.Errorf("unexpected cached content: %+v", got)
	}
}

func TestJWKSCache_InvalidateClearsCache(t *testing.T) {
	var c jwksCache
	c.set(&jwkSet{Keys: []jwk{{Kid: "k1"}}})
	c.invalidate()

	if got := c.get(); got != nil {
		t.Errorf("expected nil after invalidate, got %+v", got)
	}
}

// --- fetchJWKS ---

func TestFetchJWKS_Success(t *testing.T) {
	_, x5c := generateTestRSAKey(t)
	srv := newJWKSServer(t, "kid-1", x5c)
	defer srv.Close()

	c := newTestClient(srv, "")
	keys, err := c.fetchJWKS(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys.Keys) != 1 || keys.Keys[0].Kid != "kid-1" {
		t.Errorf("unexpected JWKS content: %+v", keys)
	}
}

func TestFetchJWKS_UsesCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(jwkSet{Keys: []jwk{{Kid: "k"}}})
	}))
	defer srv.Close()

	c := newTestClient(srv, "")
	c.fetchJWKS(context.Background())
	c.fetchJWKS(context.Background())

	if calls != 1 {
		t.Errorf("expected 1 network call (cache hit on 2nd), got %d", calls)
	}
}

func TestFetchJWKS_5xxRetriesAndFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv, "")
	_, err := c.fetchJWKS(context.Background())
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if calls != msRetryMax {
		t.Errorf("expected %d calls, got %d", msRetryMax, calls)
	}
}

// --- VerifyIDToken ---

func TestVerifyIDToken_Valid(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	c := newTestClient(srv, "test-client-id")
	tokenStr := signToken(t, key, kid, validClaims())

	claims, err := c.VerifyIDToken(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if claims.OID != "test-oid-123" {
		t.Errorf("expected OID 'test-oid-123', got %q", claims.OID)
	}
}

func TestVerifyIDToken_InvalidJWT(t *testing.T) {
	srv := newJWKSServer(t, "kid", "")
	defer srv.Close()

	c := newTestClient(srv, "")
	_, err := c.VerifyIDToken(context.Background(), "not.a.jwt")
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyIDToken_WrongAudience(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	c := newTestClient(srv, "expected-client-id")

	cl := validClaims()
	cl.Audience = jwt.ClaimStrings{"wrong-client-id"}
	tokenStr := signToken(t, key, kid, cl)

	_, err := c.VerifyIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyIDToken_EmptyOID(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	c := newTestClient(srv, "test-client-id")

	cl := validClaims()
	cl.OID = ""
	tokenStr := signToken(t, key, kid, cl)

	_, err := c.VerifyIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyIDToken_NoEmail(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	c := newTestClient(srv, "test-client-id")

	cl := validClaims()
	cl.Email = ""
	cl.PreferredUsername = ""
	tokenStr := signToken(t, key, kid, cl)

	_, err := c.VerifyIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}
