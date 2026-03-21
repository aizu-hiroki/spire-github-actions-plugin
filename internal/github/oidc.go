// Package github provides utilities for fetching and validating
// GitHub Actions OIDC tokens used for SPIRE node attestation.
package github

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	// EnvActionsIDTokenRequestURL is the env var that holds the URL
	// for requesting a GitHub Actions OIDC token.
	EnvActionsIDTokenRequestURL = "ACTIONS_ID_TOKEN_REQUEST_URL"

	// EnvActionsIDTokenRequestToken is the env var that holds the
	// bearer token used to authenticate the OIDC token request.
	EnvActionsIDTokenRequestToken = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"

	// DefaultIssuer is GitHub Actions OIDC issuer URL.
	DefaultIssuer = "https://token.actions.githubusercontent.com"

	// jwksPath is the path to the JWKS endpoint.
	jwksPath = "/.well-known/jwks"

	// requestTimeout is the HTTP request timeout for OIDC operations.
	requestTimeout = 10 * time.Second
)

// TokenResponse is the JSON response from GitHub's OIDC token endpoint.
type TokenResponse struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// AudienceClaim handles the JWT "aud" field which may be encoded as either a
// plain string (real GitHub Actions OIDC tokens) or a JSON array (jwtv4 library
// default when using ClaimStrings in tests).
type AudienceClaim []string

// UnmarshalJSON accepts both `"value"` and `["value"]`.
func (a *AudienceClaim) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*a = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*a = []string{s}
	return nil
}

// Claims holds the parsed claims from a GitHub Actions OIDC token.
type Claims struct {
	// Standard JWT claims
	Issuer   string        `json:"iss"`
	Subject  string        `json:"sub"`
	Audience AudienceClaim `json:"aud"`
	IssuedAt int64         `json:"iat"`
	Expiry   int64         `json:"exp"`

	// GitHub Actions specific claims
	Repository          string `json:"repository"`
	RepositoryID        string `json:"repository_id"`
	RepositoryOwner     string `json:"repository_owner"`
	RepositoryOwnerID   string `json:"repository_owner_id"`
	RepositoryVisibility string `json:"repository_visibility"`
	Workflow            string `json:"workflow"`
	WorkflowRef         string `json:"workflow_ref"`
	WorkflowSHA         string `json:"workflow_sha"`
	JobWorkflowRef      string `json:"job_workflow_ref"`
	JobWorkflowSHA      string `json:"job_workflow_sha"`
	Ref                 string `json:"ref"`
	RefType             string `json:"ref_type"`
	RefProtected        string `json:"ref_protected"`
	SHA                 string `json:"sha"`
	HeadRef             string `json:"head_ref"`
	BaseRef             string `json:"base_ref"`
	EventName           string `json:"event_name"`
	Actor               string `json:"actor"`
	ActorID             string `json:"actor_id"`
	RunID               string `json:"run_id"`
	RunNumber           string `json:"run_number"`
	RunAttempt          string `json:"run_attempt"`
	Environment         string `json:"environment"`
	EnvironmentNodeID   string `json:"environment_node_id"`
	RunnerEnvironment   string `json:"runner_environment"`
}

// AttestationDataWrapper is the JSON payload exchanged between the agent-side
// and server-side plugins during node attestation.
type AttestationDataWrapper struct {
	// Token is the raw GitHub Actions OIDC JWT.
	Token string `json:"token"`
}

// FetchOIDCToken requests a GitHub Actions OIDC token with the specified audience.
// It reads ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN
// from the environment.
func FetchOIDCToken(ctx context.Context, audience string) (string, error) {
	requestURL := os.Getenv(EnvActionsIDTokenRequestURL)
	if requestURL == "" {
		return "", fmt.Errorf(
			"environment variable %s is not set; "+
				"ensure the workflow has id-token: write permission",
			EnvActionsIDTokenRequestURL,
		)
	}

	requestToken := os.Getenv(EnvActionsIDTokenRequestToken)
	if requestToken == "" {
		return "", fmt.Errorf(
			"environment variable %s is not set",
			EnvActionsIDTokenRequestToken,
		)
	}

	// Append audience query parameter.
	tokenURL, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("invalid OIDC token request URL: %w", err)
	}
	q := tokenURL.Query()
	q.Set("audience", audience)
	tokenURL.RawQuery = q.Encode()

	client := &http.Client{Timeout: requestTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC token request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request OIDC token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OIDC token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"OIDC token request returned HTTP %d: %s",
			resp.StatusCode, string(body),
		)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse OIDC token response: %w", err)
	}

	if tokenResp.Value == "" {
		return "", fmt.Errorf("OIDC token response contained an empty token")
	}

	return tokenResp.Value, nil
}

// NewHTTPClient returns an http.Client for fetching JWKS or other resources.
// skipVerify should only be set true in tests.
func NewHTTPClient(skipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test only
	}
	return &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
	}
}
