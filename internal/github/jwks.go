package github

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	jwtv4 "github.com/golang-jwt/jwt/v4"
)

// jwksCache caches a JWKS response with its expiry.
type jwksCache struct {
	keys   map[string]*rsa.PublicKey
	expiry time.Time
}

// JWKSet is the JSON representation of a JWK set.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWK is a single JSON Web Key.
type JWK struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	N         string `json:"n"` // RSA modulus (base64url)
	E         string `json:"e"` // RSA exponent (base64url)
}

// TokenValidator validates GitHub Actions OIDC tokens.
type TokenValidator struct {
	issuerURL  string
	httpClient *http.Client
	cache      *jwksCache
}

// NewTokenValidator creates a new TokenValidator for the given issuer URL.
func NewTokenValidator(issuerURL string, httpClient *http.Client) *TokenValidator {
	if httpClient == nil {
		httpClient = NewHTTPClient(false)
	}
	return &TokenValidator{
		issuerURL:  issuerURL,
		httpClient: httpClient,
	}
}

// ValidateToken validates a GitHub Actions OIDC JWT and returns its claims.
// It verifies the signature using GitHub's public JWKS, validates the issuer,
// and checks that the token has not expired.
func (v *TokenValidator) ValidateToken(ctx context.Context, rawToken string, expectedAudience string) (*Claims, error) {
	// Parse the token without verifying to extract the key ID from the header.
	unverified, _, err := new(jwtv4.Parser).ParseUnverified(rawToken, jwtv4.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token header: %w", err)
	}

	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("token header missing or invalid 'kid' field")
	}

	// Fetch the public key for this key ID.
	pubKey, err := v.getPublicKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key for kid=%q: %w", kid, err)
	}

	// Parse and verify the full token.
	claims := &Claims{}
	token, err := jwtv4.ParseWithClaims(rawToken, claims, func(t *jwtv4.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwtv4.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	},
		jwtv4.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	// Validate issuer.
	if claims.Issuer != v.issuerURL {
		return nil, fmt.Errorf("unexpected issuer %q (expected %q)", claims.Issuer, v.issuerURL)
	}

	// Validate audience when specified.
	if expectedAudience != "" && claims.Audience != expectedAudience {
		return nil, fmt.Errorf("unexpected audience %q (expected %q)", claims.Audience, expectedAudience)
	}

	return claims, nil
}

// getPublicKey returns the RSA public key for the given key ID.
// Results are cached for 1 hour.
func (v *TokenValidator) getPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Return from cache if still valid.
	if v.cache != nil && time.Now().Before(v.cache.expiry) {
		if key, ok := v.cache.keys[kid]; ok {
			return key, nil
		}
	}

	// Refresh JWKS.
	jwksURL := strings.TrimRight(v.issuerURL, "/") + jwksPath
	keys, err := v.fetchJWKS(ctx, jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURL, err)
	}

	v.cache = &jwksCache{
		keys:   keys,
		expiry: time.Now().Add(time.Hour),
	}

	key, ok := keys[kid]
	if !ok {
		return nil, fmt.Errorf("no key found for kid=%q in JWKS", kid)
	}
	return key, nil
}

// fetchJWKS retrieves and parses the JWKS from the given URL.
func (v *TokenValidator) fetchJWKS(ctx context.Context, jwksURL string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching JWKS: %s", resp.StatusCode, string(body))
	}

	var jwkSet JWKSet
	if err := json.Unmarshal(body, &jwkSet); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwkSet.Keys))
	for _, jwk := range jwkSet.Keys {
		if jwk.KeyType != "RSA" {
			continue
		}
		pubKey, err := parseRSAPublicKey(jwk)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JWK kid=%q: %w", jwk.KeyID, err)
		}
		keys[jwk.KeyID] = pubKey
	}

	return keys, nil
}

// parseRSAPublicKey converts a JWK into an *rsa.PublicKey.
func parseRSAPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// Valid implements the jwt.Claims interface for Claims.
func (c *Claims) Valid() error {
	now := time.Now().Unix()
	if c.Expiry > 0 && now > c.Expiry {
		return fmt.Errorf("token has expired")
	}
	if c.IssuedAt > 0 && now < c.IssuedAt-60 {
		return fmt.Errorf("token used before issued")
	}
	return nil
}
