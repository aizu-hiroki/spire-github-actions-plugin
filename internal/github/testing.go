package github

import (
	"crypto/rsa"
	"time"
)

// NewTokenValidatorWithKey creates a TokenValidator that uses a pre-loaded
// RSA public key for signature verification instead of fetching JWKS over the
// network.  This is intended for unit tests only.
func NewTokenValidatorWithKey(issuerURL, kid string, publicKey *rsa.PublicKey) *TokenValidator {
	v := &TokenValidator{
		issuerURL:  issuerURL,
		httpClient: NewHTTPClient(false),
	}
	// Pre-populate the JWKS cache so no network requests are made during tests.
	v.cache = &jwksCache{
		keys: map[string]*rsa.PublicKey{kid: publicKey},
		// Set a far-future expiry so the cache entry is never refreshed.
		expiry: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	return v
}
