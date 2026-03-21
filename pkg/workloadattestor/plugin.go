// Package workloadattestor implements the SPIRE agent-side GitHub Actions
// workload attestation plugin.
//
// This plugin runs inside the SPIRE agent process and is invoked whenever a
// workload calls the SPIRE Workload API.  Given the workload's PID the plugin
// reads the process environment (via /proc/<pid>/environ on Linux) and looks
// for GitHub Actions context variables.  If found, it returns a set of
// selectors that describe the GitHub Actions context.
//
// This plugin is most useful when workloads (containers, scripts) are started
// inside a GitHub Actions runner and need SVID certificates tied to the
// specific workflow that launched them.
//
// Supported platforms: Linux only (requires /proc filesystem).
//
// HCL configuration example (spire-agent.conf):
//
//	WorkloadAttestor "github_actions" {
//	  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-workload"
//	  plugin_data {
//	    # Optional: emit extra env vars as selectors.
//	    extra_env_vars = ["MY_CUSTOM_VAR"]
//	  }
//	}
package workloadattestor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/hcl"
	githuboidc "github.com/aizu-hiroki/spire-github-actions-plugin/internal/github"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	workloadattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/workloadattestor/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// githubEnvVars maps GitHub Actions environment variable names to the selector
// key that will be emitted.  Only the variables listed here produce selectors.
var githubEnvVars = map[string]string{
	"GITHUB_REPOSITORY":        "repository",
	"GITHUB_REPOSITORY_OWNER":  "repository_owner",
	"GITHUB_WORKFLOW":          "workflow",
	"GITHUB_WORKFLOW_REF":      "workflow_ref",
	"GITHUB_JOB":               "job",
	"GITHUB_REF":               "ref",
	"GITHUB_REF_TYPE":          "ref_type",
	"GITHUB_SHA":               "sha",
	"GITHUB_HEAD_REF":          "head_ref",
	"GITHUB_BASE_REF":          "base_ref",
	"GITHUB_EVENT_NAME":        "event_name",
	"GITHUB_ACTOR":             "actor",
	"GITHUB_RUN_ID":            "run_id",
	"GITHUB_RUN_NUMBER":        "run_number",
	"GITHUB_RUN_ATTEMPT":       "run_attempt",
	"GITHUB_ENVIRONMENT":       "environment",
	"RUNNER_ENVIRONMENT":       "runner_environment",
	"RUNNER_OS":                "runner_os",
	"RUNNER_ARCH":              "runner_arch",
}

// pluginConfig holds the parsed HCL configuration for the workload attestor.
type pluginConfig struct {
	// ExtraEnvVars is an optional list of additional environment variable names
	// whose values should be emitted as selectors with the key "env:<VAR_NAME>".
	ExtraEnvVars []string `hcl:"extra_env_vars"`
}

// Plugin is the GitHub Actions workload attestation plugin.
type Plugin struct {
	workloadattestorv1.UnimplementedWorkloadAttestorServer
	configv1.UnimplementedConfigServer

	mu  sync.RWMutex
	cfg *pluginConfig

	// procEnvReader is used to read /proc/<pid>/environ; replaced in tests.
	procEnvReader func(pid int32) (map[string]string, error)
}

// New creates a new workload attestation plugin instance.
func New() *Plugin {
	p := &Plugin{
		cfg: &pluginConfig{},
	}
	p.procEnvReader = readProcEnviron
	return p
}

// Configure parses the plugin HCL and stores the configuration.
func (p *Plugin) Configure(_ context.Context, req *configv1.ConfigureRequest) (*configv1.ConfigureResponse, error) {
	cfg := &pluginConfig{}

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

// Attest is called by the SPIRE agent when a workload connects to the
// Workload API.  It inspects the workload process environment for GitHub
// Actions context variables and returns matching selectors.
func (p *Plugin) Attest(_ context.Context, req *workloadattestorv1.AttestRequest) (*workloadattestorv1.AttestResponse, error) {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()

	env, err := p.procEnvReader(req.Pid)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to read environment for PID %d: %v", req.Pid, err)
	}

	// Only proceed if this looks like a GitHub Actions process.
	if _, ok := env["GITHUB_ACTIONS"]; !ok {
		// Not a GitHub Actions process – return no selectors.
		return &workloadattestorv1.AttestResponse{}, nil
	}

	var selectorValues []string

	// Emit selectors for standard GitHub Actions env vars.
	for envVar, selectorKey := range githubEnvVars {
		if value, ok := env[envVar]; ok && value != "" {
			selectorValues = append(selectorValues, fmt.Sprintf("%s:%s", selectorKey, value))
		}
	}

	// Emit selectors for any additional env vars configured by the operator.
	for _, extraVar := range cfg.ExtraEnvVars {
		if value, ok := env[extraVar]; ok && value != "" {
			selectorValues = append(selectorValues,
				fmt.Sprintf("env:%s:%s", extraVar, value))
		}
	}

	// Prepend the plugin name to satisfy SPIRE's "type:value" format.
	// The SPIRE agent prepends the plugin name automatically, so we only
	// need to return the value portion.
	_ = githuboidc.PluginName // referenced to prevent unused import

	return &workloadattestorv1.AttestResponse{SelectorValues: selectorValues}, nil
}

// readProcEnviron reads the environment of a process from /proc/<pid>/environ.
// This is Linux-specific.
func readProcEnviron(pid int32) (map[string]string, error) {
	path := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return parseEnviron(data), nil
}

// parseEnviron parses the null-byte-delimited key=value pairs in
// /proc/<pid>/environ into a map.
func parseEnviron(data []byte) map[string]string {
	env := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i, b := range data {
			if b == 0 {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		entry := scanner.Text()
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}
