package auth

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
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

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

// signMSToken creates a signed RS256 JWT with the given key, kid, and claims.
func signMSToken(t *testing.T, key *rsa.PrivateKey, kid string, claims msClaims) string {
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
	jwks := msJWKSet{Keys: []msJWK{{Kid: kid, X5C: []string{x5c}}}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jwks)
	}))
}

// validMSClaims returns a minimal set of valid Microsoft ID token claims.
func validMSClaims() msClaims {
	return msClaims{
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

// newMSService builds a Service whose httpClient hits the given server.
func newMSService(srv *httptest.Server, clientID string) *Service {
	return &Service{
		microsoftClientID: clientID,
		httpClient:        &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}},
	}
}

// --- msJWKSCache ---

func TestMsJWKSCache_SetAndGet(t *testing.T) {
	var c msJWKSCache
	keys := &msJWKSet{Keys: []msJWK{{Kid: "k1"}}}
	c.set(keys)

	got := c.get()
	if got == nil {
		t.Fatal("expected cached keys, got nil")
	}
	if len(got.Keys) != 1 || got.Keys[0].Kid != "k1" {
		t.Errorf("unexpected cached content: %+v", got)
	}
}

func TestMsJWKSCache_InvalidateClearsCache(t *testing.T) {
	var c msJWKSCache
	c.set(&msJWKSet{Keys: []msJWK{{Kid: "k1"}}})
	c.invalidate()

	if got := c.get(); got != nil {
		t.Errorf("expected nil after invalidate, got %+v", got)
	}
}

// --- fetchMicrosoftJWKS ---

func TestFetchMicrosoftJWKS_Success(t *testing.T) {
	_, x5c := generateTestRSAKey(t)
	srv := newJWKSServer(t, "kid-1", x5c)
	defer srv.Close()

	svc := newMSService(srv, "")
	jwks, err := svc.fetchMicrosoftJWKS(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jwks.Keys) != 1 || jwks.Keys[0].Kid != "kid-1" {
		t.Errorf("unexpected JWKS content: %+v", jwks)
	}
}

func TestFetchMicrosoftJWKS_UsesCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(msJWKSet{Keys: []msJWK{{Kid: "k"}}})
	}))
	defer srv.Close()

	svc := newMSService(srv, "")
	svc.fetchMicrosoftJWKS(context.Background())
	svc.fetchMicrosoftJWKS(context.Background())

	if calls != 1 {
		t.Errorf("expected 1 network call (cache hit on 2nd), got %d", calls)
	}
}

func TestFetchMicrosoftJWKS_5xxRetriesAndFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := newMSService(srv, "")
	_, err := svc.fetchMicrosoftJWKS(context.Background())
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if calls != msRetryMax {
		t.Errorf("expected %d calls, got %d", msRetryMax, calls)
	}
}

// --- verifyMicrosoftIDToken ---

func TestVerifyMicrosoftIDToken_Valid(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	svc := newMSService(srv, "test-client-id")
	tokenStr := signMSToken(t, key, kid, validMSClaims())

	claims, err := svc.verifyMicrosoftIDToken(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if claims.OID != "test-oid-123" {
		t.Errorf("expected OID 'test-oid-123', got %q", claims.OID)
	}
}

func TestVerifyMicrosoftIDToken_InvalidJWT(t *testing.T) {
	srv := newJWKSServer(t, "kid", "")
	defer srv.Close()

	svc := newMSService(srv, "")
	_, err := svc.verifyMicrosoftIDToken(context.Background(), "not.a.jwt")
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyMicrosoftIDToken_WrongAudience(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	svc := newMSService(srv, "expected-client-id")

	c := validMSClaims()
	c.Audience = jwt.ClaimStrings{"wrong-client-id"}
	tokenStr := signMSToken(t, key, kid, c)

	_, err := svc.verifyMicrosoftIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyMicrosoftIDToken_EmptyOID(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	svc := newMSService(srv, "test-client-id")

	c := validMSClaims()
	c.OID = ""
	tokenStr := signMSToken(t, key, kid, c)

	_, err := svc.verifyMicrosoftIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}

func TestVerifyMicrosoftIDToken_NoEmail(t *testing.T) {
	key, x5c := generateTestRSAKey(t)
	kid := "test-kid"
	srv := newJWKSServer(t, kid, x5c)
	defer srv.Close()

	svc := newMSService(srv, "test-client-id")

	c := validMSClaims()
	c.Email = ""
	c.PreferredUsername = ""
	tokenStr := signMSToken(t, key, kid, c)

	_, err := svc.verifyMicrosoftIDToken(context.Background(), tokenStr)
	assertAPIError(t, err, "INVALID_MICROSOFT_TOKEN")
}
