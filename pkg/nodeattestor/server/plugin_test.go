package server_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"testing"
	"time"

	jwtv4 "github.com/golang-jwt/jwt/v4"
	githuboidc "github.com/aizu-hiroki/spire-github-actions-plugin/internal/github"
	"github.com/aizu-hiroki/spire-github-actions-plugin/pkg/nodeattestor/server"
	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAttestStream simulates the gRPC streaming interface for testing.
type fakeAttestStream struct {
	nodeattestorv1.NodeAttestor_AttestServer
	requests  []*nodeattestorv1.AttestRequest
	responses []*nodeattestorv1.AttestResponse
	recvIdx   int
}

func (s *fakeAttestStream) Recv() (*nodeattestorv1.AttestRequest, error) {
	if s.recvIdx >= len(s.requests) {
		return nil, io.EOF
	}
	req := s.requests[s.recvIdx]
	s.recvIdx++
	return req, nil
}

func (s *fakeAttestStream) Send(resp *nodeattestorv1.AttestResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func (s *fakeAttestStream) Context() context.Context {
	return context.Background()
}

// testTokenHelper provides helpers for generating test JWTs.
type testTokenHelper struct {
	privateKey *rsa.PrivateKey
	kid        string
}

func newTestTokenHelper(t *testing.T) *testTokenHelper {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return &testTokenHelper{privateKey: key, kid: "test-key-id-1"}
}

type testClaims struct {
	jwtv4.RegisteredClaims
	Repository      string `json:"repository"`
	RepositoryOwner string `json:"repository_owner"`
	Workflow        string `json:"workflow"`
	Ref             string `json:"ref"`
	RefType         string `json:"ref_type"`
	EventName       string `json:"event_name"`
	Actor           string `json:"actor"`
	RunID           string `json:"run_id"`
}

func (h *testTokenHelper) sign(t *testing.T, c *githuboidc.Claims) string {
	t.Helper()
	return h.signWithExpiry(t, c, time.Now().Add(time.Hour))
}

func (h *testTokenHelper) signExpired(t *testing.T, c *githuboidc.Claims) string {
	t.Helper()
	return h.signWithExpiry(t, c, time.Now().Add(-time.Hour))
}

func (h *testTokenHelper) signWithExpiry(t *testing.T, c *githuboidc.Claims, expiresAt time.Time) string {
	t.Helper()

	now := time.Now()
	claims := testClaims{
		RegisteredClaims: jwtv4.RegisteredClaims{
			Issuer:    c.Issuer,
			Subject:   c.Subject,
			Audience:  jwtv4.ClaimStrings(c.Audience),
			IssuedAt:  jwtv4.NewNumericDate(now),
			ExpiresAt: jwtv4.NewNumericDate(expiresAt),
		},
		Repository:      c.Repository,
		RepositoryOwner: c.RepositoryOwner,
		Workflow:        c.Workflow,
		Ref:             c.Ref,
		RefType:         c.RefType,
		EventName:       c.EventName,
		Actor:           c.Actor,
		RunID:           c.RunID,
	}

	token := jwtv4.NewWithClaims(jwtv4.SigningMethodRS256, claims)
	token.Header["kid"] = h.kid

	signed, err := token.SignedString(h.privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

// mustAttest is a helper that builds a stream with a single payload and calls Attest.
func mustAttest(plug *server.Plugin, payload []byte) error {
	stream := &fakeAttestStream{
		requests: []*nodeattestorv1.AttestRequest{
			{Request: &nodeattestorv1.AttestRequest_Payload{Payload: payload}},
		},
	}
	return plug.Attest(stream)
}

// validClaims returns a minimal set of valid claims for tests.
func validClaims() *githuboidc.Claims {
	return &githuboidc.Claims{
		Issuer:          "https://token.actions.githubusercontent.com",
		Subject:         "repo:my-org/my-repo:ref:refs/heads/main",
		Audience:        []string{"spire-server"},
		Repository:      "my-org/my-repo",
		RepositoryOwner: "my-org",
	}
}

// configuredPlugin returns a plugin configured with the given helper's public key.
func configuredPlugin(t *testing.T, helper *testTokenHelper, hcl string) *server.Plugin {
	t.Helper()
	plug := server.New()
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{
		HclConfiguration:  hcl,
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: "example.org"},
	})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	plug.SetValidatorForTest(githuboidc.NewTokenValidatorWithKey(
		"https://token.actions.githubusercontent.com",
		helper.kid,
		&helper.privateKey.PublicKey,
	))
	return plug
}

// --- Tests ---

func TestAttest_Success(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["my-org"]
	`)

	rawToken := helper.sign(t, &githuboidc.Claims{
		Issuer:          "https://token.actions.githubusercontent.com",
		Subject:         "repo:my-org/my-repo:ref:refs/heads/main",
		Audience:        []string{"spire-server"},
		Repository:      "my-org/my-repo",
		RepositoryOwner: "my-org",
		Workflow:        "CI",
		Ref:             "refs/heads/main",
		RefType:         "branch",
		EventName:       "push",
		Actor:           "octocat",
		RunID:           "1234567890",
	})

	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})
	stream := &fakeAttestStream{
		requests: []*nodeattestorv1.AttestRequest{
			{Request: &nodeattestorv1.AttestRequest_Payload{Payload: payload}},
		},
	}

	if err := plug.Attest(stream); err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	attrs := stream.responses[0].GetAgentAttributes()
	if attrs == nil {
		t.Fatal("expected AgentAttributes in response")
	}

	wantAgentID := "spiffe://example.org/spire/agent/github_actions/my-org/my-repo"
	if attrs.SpiffeId != wantAgentID {
		t.Errorf("SpiffeId = %q, want %q", attrs.SpiffeId, wantAgentID)
	}

	selectorSet := make(map[string]bool, len(attrs.SelectorValues))
	for _, v := range attrs.SelectorValues {
		selectorSet[v] = true
	}

	for _, want := range []string{
		"repository:my-org/my-repo",
		"repository_owner:my-org",
		"workflow:CI",
		"ref:refs/heads/main",
		"ref_type:branch",
		"event_name:push",
		"actor:octocat",
	} {
		if !selectorSet[want] {
			t.Errorf("missing selector value %q", want)
		}
	}
}

func TestAttest_AllowedOwnerRejected(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["trusted-org"]
	`)

	rawToken := helper.sign(t, &githuboidc.Claims{
		Issuer:          "https://token.actions.githubusercontent.com",
		Subject:         "repo:untrusted-org/repo:ref:refs/heads/main",
		Audience:        []string{"spire-server"},
		Repository:      "untrusted-org/repo",
		RepositoryOwner: "untrusted-org",
	})

	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})
	stream := &fakeAttestStream{
		requests: []*nodeattestorv1.AttestRequest{
			{Request: &nodeattestorv1.AttestRequest_Payload{Payload: payload}},
		},
	}

	if err := plug.Attest(stream); err == nil {
		t.Fatal("expected error for disallowed owner, got nil")
	} else if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_AllowedRepositoryAccepted(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repositories = ["my-org/my-repo"]
	`)

	rawToken := helper.sign(t, validClaims())
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err != nil {
		t.Fatalf("expected success for allowed repository, got %v", err)
	}
}

func TestAttest_AllowedRepositoryRejected(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repositories = ["my-org/my-repo"]
	`)

	claims := validClaims()
	claims.Repository = "my-org/other-repo"
	rawToken := helper.sign(t, claims)
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err == nil {
		t.Fatal("expected error for disallowed repository, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_EmptyPayload(t *testing.T) {
	plug := server.New()
	stream := &fakeAttestStream{
		requests: []*nodeattestorv1.AttestRequest{
			{Request: &nodeattestorv1.AttestRequest_Payload{Payload: nil}},
		},
	}
	err := plug.Attest(stream)
	if err == nil {
		t.Fatal("expected error for empty payload, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestAttest_InvalidSignature(t *testing.T) {
	helper := newTestTokenHelper(t)
	wrongHelper := newTestTokenHelper(t) // different RSA key pair

	// Validator uses helper's public key, but token is signed with wrongHelper's key.
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["my-org"]
	`)

	rawToken := wrongHelper.sign(t, validClaims())
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_ExpiredToken(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["my-org"]
	`)

	rawToken := helper.signExpired(t, validClaims())
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_WrongIssuer(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["my-org"]
	`)

	claims := validClaims()
	claims.Issuer = "https://evil.example.com"
	rawToken := helper.sign(t, claims)
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_WrongAudience(t *testing.T) {
	helper := newTestTokenHelper(t)
	plug := configuredPlugin(t, helper, `
		audience = "spire-server"
		allowed_repository_owners = ["my-org"]
	`)

	claims := validClaims()
	claims.Audience = []string{"wrong-audience"}
	rawToken := helper.sign(t, claims)
	payload, _ := json.Marshal(&githuboidc.AttestationDataWrapper{Token: rawToken})

	err := mustAttest(plug, payload)
	if err == nil {
		t.Fatal("expected error for wrong audience, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestAttest_InvalidJSON(t *testing.T) {
	plug := server.New()

	err := mustAttest(plug, []byte("not-valid-json{{{"))
	if err == nil {
		t.Fatal("expected error for invalid JSON payload, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestConfigure_EmptyAudience(t *testing.T) {
	plug := server.New()
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{
		HclConfiguration: `
			audience = ""
			allowed_repository_owners = ["my-org"]
		`,
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: "example.org"},
	})
	if err == nil {
		t.Fatal("expected error for empty audience, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestConfigure_NoAllowList(t *testing.T) {
	plug := server.New()
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{
		HclConfiguration:  `audience = "spire-server"`,
		CoreConfiguration: &configv1.CoreConfiguration{TrustDomain: "example.org"},
	})
	if err == nil {
		t.Fatal("expected error when no allow-list is configured, got nil")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}
