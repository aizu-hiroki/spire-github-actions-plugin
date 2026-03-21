// Command workloadattestor is the SPIRE agent-side GitHub Actions workload
// attestation plugin binary.
//
// Configure it in spire-agent.conf:
//
//	WorkloadAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-workload"
//	  plugin_data {
//	    # Optional: emit extra env vars as selectors.
//	    extra_env_vars = ["MY_CUSTOM_VAR"]
//	  }
//	}
package main

import (
	workloadplugin "github.com/aizu-hiroki/spire-github-actions-plugin/pkg/workloadattestor"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	workloadattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/workloadattestor/v1"
	"github.com/spiffe/spire-plugin-sdk/pluginmain"
)

func main() {
	plugin := workloadplugin.New()
	pluginmain.Serve(
		workloadattestorv1.WorkloadAttestorPluginServer(plugin),
		configv1.ConfigServiceServer(plugin),
	)
}
