package workloadattestor_test

import (
	"context"
	"testing"

	workloadplugin "github.com/aizu-hiroki/spire-github-actions-plugin/pkg/workloadattestor"
	configv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/service/common/config/v1"
	workloadattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/workloadattestor/v1"
)

func newTestPlugin(t *testing.T, environ map[string]string) *workloadplugin.Plugin {
	t.Helper()
	p := workloadplugin.New()
	p.SetProcEnvReaderForTest(func(_ int32) (map[string]string, error) {
		return environ, nil
	})
	return p
}

func TestAttest_GitHubActionsProcess(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":          "true",
		"GITHUB_REPOSITORY":       "my-org/my-repo",
		"GITHUB_REPOSITORY_OWNER": "my-org",
		"GITHUB_WORKFLOW":         "CI",
		"GITHUB_REF":              "refs/heads/main",
		"GITHUB_REF_TYPE":         "branch",
		"GITHUB_EVENT_NAME":       "push",
		"GITHUB_ACTOR":            "octocat",
		"GITHUB_RUN_ID":           "9876543210",
		"RUNNER_ENVIRONMENT":      "github-hosted",
	}

	plug := newTestPlugin(t, env)
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 1234})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	selectorSet := make(map[string]bool)
	for _, v := range resp.SelectorValues {
		selectorSet[v] = true
	}

	wantValues := []string{
		"repository:my-org/my-repo",
		"repository_owner:my-org",
		"workflow:CI",
		"ref:refs/heads/main",
		"ref_type:branch",
		"event_name:push",
		"actor:octocat",
		"run_id:9876543210",
		"runner_environment:github-hosted",
	}
	for _, want := range wantValues {
		if !selectorSet[want] {
			t.Errorf("missing selector value %q", want)
		}
	}
}

func TestAttest_NonGitHubActionsProcess(t *testing.T) {
	plug := newTestPlugin(t, map[string]string{
		"HOME": "/home/user",
		"PATH": "/usr/bin:/bin",
	})

	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 5678})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	if len(resp.SelectorValues) != 0 {
		t.Errorf("expected no selectors for non-GitHub-Actions process, got %d", len(resp.SelectorValues))
	}
}

func TestAttest_ExtraEnvVars(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_REPOSITORY": "my-org/my-repo",
		"MY_CUSTOM_VAR":     "custom-value",
	}

	plug := newTestPlugin(t, env)
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{
		HclConfiguration: `extra_env_vars = ["MY_CUSTOM_VAR"]`,
	})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 9999})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	found := false
	for _, v := range resp.SelectorValues {
		if v == "env:MY_CUSTOM_VAR:custom-value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected selector value env:MY_CUSTOM_VAR:custom-value not found")
	}
}
