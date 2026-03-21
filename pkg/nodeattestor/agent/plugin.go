// Package agent implements the SPIRE agent-side GitHub Actions node attestation plugin.
//
// This plugin runs inside the SPIRE agent process.  When the agent needs to
// attest itself to the SPIRE server it calls AidAttestation, which fetches a
// short-lived GitHub Actions OIDC token and sends its raw JWT bytes to the
// server-side counterpart for verification.
//
// Required workflow permission:
//
//	permissions:
//	  id-token: write
//
// HCL configuration example (spire-agent.conf):
//
//	NodeAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-agent-plugin-github-actions"
//	  plugin_data {
//	    audience = "spire-server"   # optional, defaults to "spire-server"
//	  }
//	}
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hashicorp/hcl"
	githuboidc "github.com/aizu-hiroki/spire-github-actions-plugin/internal/github"
	nodeattestoragentv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginConfig holds the parsed HCL configuration for the agent plugin.
type pluginConfig struct {
	// Audience is the OIDC token audience to request.
	// Defaults to "spire-server".
	Audience string `hcl:"audience"`
}

// Plugin is the agent-side GitHub Actions node attestation plugin.
// It implements the SPIRE agent NodeAttestor gRPC service.
type Plugin struct {
	nodeattestoragentv1.UnimplementedNodeAttestorServer
	configv1.UnimplementedConfigServer

	mu  sync.RWMutex
	cfg *pluginConfig
}

// New creates a new agent-side plugin instance.
func New() *Plugin {
	return &Plugin{
		cfg: &pluginConfig{
			Audience: "spire-server",
		},
	}
}

// Configure parses the plugin HCL and stores the configuration.
// It implements the SPIRE config service interface.
func (p *Plugin) Configure(_ context.Context, req *configv1.ConfigureRequest) (*configv1.ConfigureResponse, error) {
	cfg := &pluginConfig{
		Audience: "spire-server", // default
	}

	if req.HclConfiguration != "" {
		if err := hcl.Decode(cfg, req.HclConfiguration); err != nil {
			return nil, status.Errorf(codes.InvalidArgument,
				"failed to parse plugin configuration: %v", err)
		}
	}

	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()

	return &configv1.ConfigureResponse{}, nil
}

// AidAttestation is called by the SPIRE agent to perform node attestation.
// It fetches a GitHub Actions OIDC token and sends it to the server.
//
// Protocol flow:
//  1. Agent sends payload (JWT token wrapped in AttestationDataWrapper JSON)
//  2. Server optionally sends a challenge (not used in this plugin)
//  3. Server sends the final attestation result
func (p *Plugin) AidAttestation(stream nodeattestoragentv1.NodeAttestor_AidAttestationServer) error {
	p.mu.RLock()
	audience := p.cfg.Audience
	p.mu.RUnlock()

	ctx := stream.Context()

	// Step 1: Fetch the OIDC token from GitHub Actions runtime.
	token, err := githuboidc.FetchOIDCToken(ctx, audience)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition,
			"failed to fetch GitHub Actions OIDC token: %v", err)
	}

	// Step 2: Encode as attestation payload.
	payload, err := json.Marshal(&githuboidc.AttestationDataWrapper{Token: token})
	if err != nil {
		return status.Errorf(codes.Internal,
			"failed to marshal attestation data: %v", err)
	}

	// Step 3: Send payload to the server-side plugin.
	if err := stream.Send(&nodeattestoragentv1.PayloadOrChallengeResponse{
		Data: &nodeattestoragentv1.PayloadOrChallengeResponse_Payload{
			Payload: payload,
		},
	}); err != nil {
		return fmt.Errorf("failed to send attestation payload: %w", err)
	}

	// Step 4: Handle optional server challenges.
	// The GitHub Actions node attestor does not issue challenges, but we must
	// drain the stream until it closes.
	for {
		challenge, err := stream.Recv()
		if err != nil {
			// Stream ended normally after server sent the attestation result.
			return nil
		}

		// If the server sends a challenge, we cannot respond to it.
		if len(challenge.GetChallenge()) > 0 {
			return status.Error(codes.Unimplemented,
				"github_actions node attestor does not support server challenges")
		}
	}
}
