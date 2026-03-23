// Package server implements the SPIRE server-side GitHub Actions node attestation plugin.
//
// This plugin runs inside the SPIRE server process.  When a SPIRE agent sends an
// attestation request the server calls Attest, which:
//  1. Receives the raw JWT from the agent
//  2. Validates the JWT signature using GitHub's public JWKS
//  3. Verifies the issuer, audience and standard claims
//  4. Optionally restricts attestation to specific repository owners
//  5. Returns a SPIFFE agent ID and a rich set of selectors
//
// HCL configuration example (spire-server.conf):
//
//	NodeAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-server-plugin-github-actions"
//	  plugin_data {
//	    # Optional: restrict to specific GitHub organisation/user names.
//	    allowed_repository_owners = ["my-org", "my-user"]
//
//	    # Optional: expected OIDC audience.  Must match the agent plugin's
//	    # audience setting.  Defaults to "spire-server".
//	    audience = "spire-server"
//
//	    # Optional: override the OIDC issuer URL (useful for GHES).
//	    # Defaults to "https://token.actions.githubusercontent.com".
//	    oidc_issuer_url = "https://token.actions.githubusercontent.com"
//	  }
//	}
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/hcl"
	githuboidc "github.com/aizu-hiroki/spire-github-actions-plugin/internal/github"
	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginConfig holds the parsed HCL configuration for the server plugin.
type pluginConfig struct {
	// AllowedRepositoryOwners is an optional list of GitHub repository owner names
	// (organisations or users) that are allowed to attest.  If empty, all owners
	// are allowed.
	AllowedRepositoryOwners []string `hcl:"allowed_repository_owners"`

	// Audience is the expected OIDC token audience.  Must match the value
	// configured in the agent plugin.  Defaults to "spire-server".
	Audience string `hcl:"audience"`

	// OIDCIssuerURL is the expected OIDC issuer URL.
	// Defaults to "https://token.actions.githubusercontent.com".
	OIDCIssuerURL string `hcl:"oidc_issuer_url"`
}

// Plugin is the server-side GitHub Actions node attestation plugin.
// It implements the SPIRE server NodeAttestor gRPC service.
type Plugin struct {
	nodeattestorv1.UnimplementedNodeAttestorServer
	configv1.UnimplementedConfigServer

	mu          sync.RWMutex
	cfg         *pluginConfig
	trustDomain string
	validator   *githuboidc.TokenValidator
}

// New creates a new server-side plugin instance with default configuration.
func New() *Plugin {
	cfg := defaultConfig()
	return &Plugin{
		cfg:       cfg,
		validator: githuboidc.NewTokenValidator(cfg.OIDCIssuerURL, nil),
	}
}

func defaultConfig() *pluginConfig {
	return &pluginConfig{
		Audience:      "spire-server",
		OIDCIssuerURL: githuboidc.DefaultIssuer,
	}
}

// Configure parses the plugin HCL, stores the configuration, and rebuilds the
// token validator.  It implements the SPIRE config service interface.
func (p *Plugin) Configure(_ context.Context, req *configv1.ConfigureRequest) (*configv1.ConfigureResponse, error) {
	cfg := defaultConfig()

	if req.HclConfiguration != "" {
		if err := hcl.Decode(cfg, req.HclConfiguration); err != nil {
			return nil, status.Errorf(codes.InvalidArgument,
				"failed to parse plugin configuration: %v", err)
		}
	}

	p.mu.Lock()
	p.cfg = cfg
	p.trustDomain = req.CoreConfiguration.GetTrustDomain()
	p.validator = githuboidc.NewTokenValidator(cfg.OIDCIssuerURL, nil)
	p.mu.Unlock()

	return &configv1.ConfigureResponse{}, nil
}

// Attest is called by the SPIRE server when an agent presents an attestation
// request.  It validates the GitHub Actions OIDC token and returns the agent
// SPIFFE ID along with a set of selectors derived from the token claims.
func (p *Plugin) Attest(stream nodeattestorv1.NodeAttestor_AttestServer) error {
	// Step 1: Receive the attestation payload from the agent.
	req, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			return status.Error(codes.InvalidArgument, "no attestation payload received")
		}
		return fmt.Errorf("failed to receive attestation request: %w", err)
	}

	// The first message must carry the attestation payload.
	payloadBytes := req.GetPayload()
	if len(payloadBytes) == 0 {
		return status.Error(codes.InvalidArgument,
			"first attestation request message must contain a payload")
	}

	// Step 2: Unmarshal the attestation data.
	var attestData githuboidc.AttestationDataWrapper
	if err := json.Unmarshal(payloadBytes, &attestData); err != nil {
		return status.Errorf(codes.InvalidArgument,
			"failed to unmarshal attestation data: %v", err)
	}
	if attestData.Token == "" {
		return status.Error(codes.InvalidArgument,
			"attestation data does not contain a token")
	}

	p.mu.RLock()
	cfg := p.cfg
	trustDomain := p.trustDomain
	validator := p.validator
	p.mu.RUnlock()

	// Step 3: Validate the JWT.
	ctx := stream.Context()
	claims, err := validator.ValidateToken(ctx, attestData.Token, cfg.Audience)
	if err != nil {
		return status.Errorf(codes.PermissionDenied,
			"token validation failed: %v", err)
	}

	// Step 4: Enforce repository owner allow-list (if configured).
	if len(cfg.AllowedRepositoryOwners) > 0 {
		if err := checkAllowedOwner(claims.RepositoryOwner, cfg.AllowedRepositoryOwners); err != nil {
			return status.Errorf(codes.PermissionDenied, "%v", err)
		}
	}

	// Step 5: Build the SPIFFE agent ID.
	agentID := githuboidc.AgentID(trustDomain, claims.Repository)

	// Step 6: Build selector values from claims.
	// SPIRE prepends the plugin name automatically; values are "key:value" strings.
	selectorValues := githuboidc.BuildSelectors(claims)

	// Step 7: Send the attestation result back to the agent.
	if err := stream.Send(&nodeattestorv1.AttestResponse{
		Response: &nodeattestorv1.AttestResponse_AgentAttributes{
			AgentAttributes: &nodeattestorv1.AgentAttributes{
				SpiffeId:       agentID,
				SelectorValues: selectorValues,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send attestation response: %w", err)
	}

	return nil
}

// checkAllowedOwner returns an error if owner is not in the allowedOwners list.
func checkAllowedOwner(owner string, allowedOwners []string) error {
	for _, allowed := range allowedOwners {
		if owner == allowed {
			return nil
		}
	}
	return fmt.Errorf(
		"repository owner %q is not in the allowed list %v",
		owner, allowedOwners,
	)
}
