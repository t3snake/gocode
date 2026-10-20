package openaisiwc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/t3snake/gocode/src/core"
)

type idTokenClaims struct {
	Issuer    string         `json:"iss"`
	Subject   string         `json:"sub"`
	Audience  jsontext.Value `json:"aud"`
	Party     string         `json:"azp"`
	Expires   int64          `json:"exp"`
	IssuedAt  *int64         `json:"iat"`
	NotBefore *int64         `json:"nbf"`
	Nonce     string         `json:"nonce"`
	Email     string         `json:"email"`
}

type signingKey struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func fetchSigningKeys(ctx context.Context) ([]signingKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, core.OpenAiSiwcJwksEndpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch signing keys: HTTP %d", response.StatusCode)
	}
	var jwks struct {
		Keys []signingKey `json:"keys"`
	}
	if err := json.UnmarshalRead(io.LimitReader(response.Body, 1<<20), &jwks); err != nil {
		return nil, fmt.Errorf("decode signing keys: %w", err)
	}
	return jwks.Keys, nil
}

// validateIDToken only uses its inputs. It does not read the clock or make HTTP requests.
func validateIDToken(token string, signing_keys []signingKey, client_id, nonce string, now time.Time) (idTokenClaims, error) {
	var claims idTokenClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, fmt.Errorf("invalid ID token")
	}
	var header struct {
		Algorithm string         `json:"alg"`
		KeyID     string         `json:"kid"`
		Critical  jsontext.Value `json:"crit"`
	}
	header_bytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, fmt.Errorf("invalid ID-token header encoding")
	}
	if err := json.Unmarshal(header_bytes, &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return claims, fmt.Errorf("invalid ID-token header")
	}
	if header.Critical != nil {
		return claims, fmt.Errorf("unsupported critical ID-token header")
	}

	var public_key *rsa.PublicKey
	for _, key := range signing_keys {
		if key.KeyID != header.KeyID || key.KeyType != "RSA" || (key.Use != "" && key.Use != "sig") || (key.Algorithm != "" && key.Algorithm != "RS256") {
			continue
		}
		modulus, modulus_err := base64.RawURLEncoding.DecodeString(key.Modulus)
		exponent, exponent_err := base64.RawURLEncoding.DecodeString(key.Exponent)
		if modulus_err != nil || exponent_err != nil || len(exponent) == 0 || len(exponent) > 4 {
			return claims, fmt.Errorf("invalid signing key")
		}
		public_key = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(new(big.Int).SetBytes(exponent).Int64())}
		break
	}
	if public_key == nil {
		return claims, fmt.Errorf("ID-token signing key was not found")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, fmt.Errorf("invalid ID-token signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(public_key, crypto.SHA256, digest[:], signature); err != nil {
		return claims, fmt.Errorf("invalid ID-token signature")
	}

	claims_bytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, fmt.Errorf("invalid ID-token claims encoding")
	}
	if err := json.Unmarshal(claims_bytes, &claims); err != nil {
		return claims, fmt.Errorf("invalid ID-token claims")
	}
	// Accept the client as a string or a list. No other audience is trusted here.
	var audience string
	var audiences []string
	if err := json.Unmarshal(claims.Audience, &audience); err == nil && audience != "" {
		audiences = []string{audience}
	} else if err := json.Unmarshal(claims.Audience, &audiences); err != nil {
		return claims, fmt.Errorf("invalid ID-token audience")
	}
	if len(audiences) == 0 {
		return claims, fmt.Errorf("missing ID-token audience")
	}
	for _, audience := range audiences {
		if audience != client_id {
			return claims, fmt.Errorf("ID-token audience does not match the client")
		}
	}
	if claims.Issuer != "https://auth.openai.com" {
		return claims, fmt.Errorf("invalid ID-token issuer")
	}
	if claims.Expires <= now.Unix() {
		return claims, fmt.Errorf("ID token has expired")
	}
	if claims.IssuedAt == nil || *claims.IssuedAt > now.Unix() || (claims.NotBefore != nil && *claims.NotBefore > now.Unix()) {
		return claims, fmt.Errorf("invalid ID-token issued-at or not-before time")
	}
	if claims.Nonce != nonce {
		return claims, fmt.Errorf("ID-token nonce does not match this sign-in")
	}
	if claims.Subject == "" || (claims.Party != "" && claims.Party != client_id) {
		return claims, fmt.Errorf("invalid ID-token identity or authorized party")
	}
	return claims, nil
}
