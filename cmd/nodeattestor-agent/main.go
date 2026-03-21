// Command nodeattestor-agent is the SPIRE agent-side GitHub Actions node
// attestation plugin binary.
//
// Configure it in spire-agent.conf:
//
//	NodeAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-agent"
//	  plugin_data {
//	    audience = "spire-server"
//	  }
//	}
package main

import (
	agentplugin "github.com/aizu-hiroki/spire-github-actions-plugin/pkg/nodeattestor/agent"
	nodeattestoragentv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"github.com/spiffe/spire-plugin-sdk/pluginmain"
)

func main() {
	plugin := agentplugin.New()
	pluginmain.Serve(
		nodeattestoragentv1.NodeAttestorPluginServer(plugin),
		configv1.ConfigServiceServer(plugin),
	)
}
