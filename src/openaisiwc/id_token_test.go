package openaisiwc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"
)

func signedToken(key *rsa.PrivateKey, header, claims string) (string, error) {
	input := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(claims))
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func publicSigningKey(key *rsa.PrivateKey) signingKey {
	return signingKey{
		KeyID: "test", KeyType: "RSA", Use: "sig", Algorithm: "RS256",
		Modulus:  base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		Exponent: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func TestIDTokenTrust(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := []signingKey{publicSigningKey(key)}
	const valid_claims = `{"iss":"https://auth.openai.com","sub":"user","aud":"client","nonce":"attempt","iat":1000,"exp":2000}`
	test_cases := []struct {
		name        string
		old         string
		replacement string
		header      string
		tamper      bool
		want_error  string
	}{
		{name: "valid"},
		{name: "audience list", old: `"aud":"client"`, replacement: `"aud":["client"]`},
		{name: "changed signed data", tamper: true, want_error: "signature"},
		{name: "unsigned algorithm", header: `{"alg":"none","kid":"test"}`, want_error: "header"},
		{name: "unknown key", header: `{"alg":"RS256","kid":"other"}`, want_error: "key was not found"},
		{name: "unknown extension", header: `{"alg":"RS256","kid":"test","crit":["rule"]}`, want_error: "critical"},
		{name: "wrong issuer", old: "https://auth.openai.com", replacement: "https://example.invalid", want_error: "issuer"},
		{name: "wrong client", old: `"aud":"client"`, replacement: `"aud":"other"`, want_error: "audience"},
		{name: "untrusted audience", old: `"aud":"client"`, replacement: `"aud":["client","other"]`, want_error: "audience"},
		{name: "malformed audience", old: `"aud":"client"`, replacement: `"aud":[{}]`, want_error: "audience"},
		{name: "missing audience", old: `"aud":"client",`, want_error: "audience"},
		{name: "wrong attempt", old: `"nonce":"attempt"`, replacement: `"nonce":"other"`, want_error: "nonce"},
		{name: "expiry boundary", old: `"exp":2000`, replacement: `"exp":1500`, want_error: "expired"},
		{name: "future issue time", old: `"iat":1000`, replacement: `"iat":1600`, want_error: "issued-at"},
		{name: "missing issue time", old: `"iat":1000,`, want_error: "issued-at"},
		{name: "not yet valid", old: `"iat":1000`, replacement: `"iat":1000,"nbf":1600`, want_error: "not-before"},
		{name: "missing account", old: `"sub":"user",`, want_error: "identity"},
		{name: "wrong authorized party", old: `"sub":"user"`, replacement: `"sub":"user","azp":"other"`, want_error: "authorized party"},
	}
	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			claims := valid_claims
			if tt.old != "" {
				claims = strings.Replace(claims, tt.old, tt.replacement, 1)
			}
			header := tt.header
			if header == "" {
				header = `{"alg":"RS256","kid":"test"}`
			}
			token, err := signedToken(key, header, claims)
			if err != nil {
				t.Fatal(err)
			}
			if tt.tamper {
				parts := strings.Split(token, ".")
				parts[1] = base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(claims, "user", "attacker", 1)))
				token = strings.Join(parts, ".")
			}
			identity, err := validateIDToken(token, keys, "client", "attempt", time.Unix(1500, 0))
			if tt.want_error != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want_error) {
					t.Fatalf("got %v, want an error containing %q", err, tt.want_error)
				}
			} else if err != nil || identity.Subject != "user" {
				t.Fatalf("valid identity rejected: %v", err)
			}
		})
	}
}
