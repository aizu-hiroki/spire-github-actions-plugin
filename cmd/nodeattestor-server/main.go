// Command nodeattestor-server is the SPIRE server-side GitHub Actions node
// attestation plugin binary.
//
// Configure it in spire-server.conf:
//
//	NodeAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-server"
//	  plugin_data {
//	    allowed_repository_owners = ["my-org"]
//	    audience                  = "spire-server"
//	  }
//	}
package main

import (
	serverplugin "github.com/aizu-hiroki/spire-github-actions-plugin/pkg/nodeattestor/server"
	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	"github.com/spiffe/spire-plugin-sdk/pluginmain"
)

func main() {
	plugin := serverplugin.New()
	pluginmain.Serve(
		nodeattestorv1.NodeAttestorPluginServer(plugin),
		configv1.ConfigServiceServer(plugin),
	)
}
