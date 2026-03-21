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

func selectorSet(resp *workloadattestorv1.AttestResponse) map[string]bool {
	set := make(map[string]bool, len(resp.SelectorValues))
	for _, v := range resp.SelectorValues {
		set[v] = true
	}
	return set
}

// TestAttest_AllStandardSelectors verifies that every supported GitHub Actions
// environment variable produces the expected selector value.
func TestAttest_AllStandardSelectors(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":          "true",
		"GITHUB_REPOSITORY":       "my-org/my-repo",
		"GITHUB_REPOSITORY_OWNER": "my-org",
		"GITHUB_WORKFLOW":         "CI",
		"GITHUB_WORKFLOW_REF":     "my-org/my-repo/.github/workflows/ci.yml@refs/heads/main",
		"GITHUB_JOB":              "test",
		"GITHUB_REF":              "refs/heads/main",
		"GITHUB_REF_TYPE":         "branch",
		"GITHUB_SHA":              "abc123def456",
		"GITHUB_EVENT_NAME":       "push",
		"GITHUB_ACTOR":            "octocat",
		"GITHUB_RUN_ID":           "9876543210",
		"GITHUB_RUN_NUMBER":       "42",
		"GITHUB_RUN_ATTEMPT":      "1",
		"RUNNER_ENVIRONMENT":      "github-hosted",
		"RUNNER_OS":               "Linux",
		"RUNNER_ARCH":             "X64",
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

	selectors := selectorSet(resp)

	wantValues := []string{
		"repository:my-org/my-repo",
		"repository_owner:my-org",
		"workflow:CI",
		"workflow_ref:my-org/my-repo/.github/workflows/ci.yml@refs/heads/main",
		"job:test",
		"ref:refs/heads/main",
		"ref_type:branch",
		"sha:abc123def456",
		"event_name:push",
		"actor:octocat",
		"run_id:9876543210",
		"run_number:42",
		"run_attempt:1",
		"runner_environment:github-hosted",
		"runner_os:Linux",
		"runner_arch:X64",
	}
	for _, want := range wantValues {
		if !selectors[want] {
			t.Errorf("missing selector value %q", want)
		}
	}
}

// TestAttest_PullRequestSelectors verifies head_ref and base_ref selectors
// which are only set for pull_request events.
func TestAttest_PullRequestSelectors(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_REPOSITORY": "my-org/my-repo",
		"GITHUB_EVENT_NAME": "pull_request",
		"GITHUB_HEAD_REF":   "feature/my-branch",
		"GITHUB_BASE_REF":   "main",
	}

	plug := newTestPlugin(t, env)
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 2222})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	selectors := selectorSet(resp)
	for _, want := range []string{
		"head_ref:feature/my-branch",
		"base_ref:main",
	} {
		if !selectors[want] {
			t.Errorf("missing selector value %q", want)
		}
	}
}

// TestAttest_DeploymentEnvironmentSelector verifies the environment selector
// which is only set for deployment jobs.
func TestAttest_DeploymentEnvironmentSelector(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "my-org/my-repo",
		"GITHUB_ENVIRONMENT":  "production",
	}

	plug := newTestPlugin(t, env)
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 3333})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	selectors := selectorSet(resp)
	if !selectors["environment:production"] {
		t.Error("missing selector value \"environment:production\"")
	}
}

// TestAttest_EmptyEnvVarsOmitted verifies that env vars present but empty
// do not produce selectors.
func TestAttest_EmptyEnvVarsOmitted(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_REPOSITORY": "my-org/my-repo",
		"GITHUB_SHA":        "", // empty — should be omitted
	}

	plug := newTestPlugin(t, env)
	_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 4444})
	if err != nil {
		t.Fatalf("Attest failed: %v", err)
	}

	selectors := selectorSet(resp)
	for k := range selectors {
		if k == "sha:" || k == "sha" {
			t.Errorf("empty env var produced unexpected selector %q", k)
		}
	}
}

// TestAttest_AbsentEnvVarProducesNoSelector verifies that when a specific
// GitHub Actions env var is absent, its corresponding selector is not emitted.
func TestAttest_AbsentEnvVarProducesNoSelector(t *testing.T) {
	fullEnv := map[string]string{
		"GITHUB_ACTIONS":          "true",
		"GITHUB_REPOSITORY":       "my-org/my-repo",
		"GITHUB_REPOSITORY_OWNER": "my-org",
		"GITHUB_WORKFLOW":         "CI",
		"GITHUB_WORKFLOW_REF":     "my-org/my-repo/.github/workflows/ci.yml@refs/heads/main",
		"GITHUB_JOB":              "test",
		"GITHUB_REF":              "refs/heads/main",
		"GITHUB_REF_TYPE":         "branch",
		"GITHUB_SHA":              "abc123",
		"GITHUB_HEAD_REF":         "feature/x",
		"GITHUB_BASE_REF":         "main",
		"GITHUB_EVENT_NAME":       "push",
		"GITHUB_ACTOR":            "octocat",
		"GITHUB_RUN_ID":           "1",
		"GITHUB_RUN_NUMBER":       "2",
		"GITHUB_RUN_ATTEMPT":      "3",
		"GITHUB_ENVIRONMENT":      "production",
		"RUNNER_ENVIRONMENT":      "github-hosted",
		"RUNNER_OS":               "Linux",
		"RUNNER_ARCH":             "X64",
	}

	tests := []struct {
		envVar          string
		selectorPrefix  string
	}{
		{"GITHUB_REPOSITORY", "repository:"},
		{"GITHUB_REPOSITORY_OWNER", "repository_owner:"},
		{"GITHUB_WORKFLOW", "workflow:"},
		{"GITHUB_WORKFLOW_REF", "workflow_ref:"},
		{"GITHUB_JOB", "job:"},
		{"GITHUB_REF", "ref:"},
		{"GITHUB_REF_TYPE", "ref_type:"},
		{"GITHUB_SHA", "sha:"},
		{"GITHUB_HEAD_REF", "head_ref:"},
		{"GITHUB_BASE_REF", "base_ref:"},
		{"GITHUB_EVENT_NAME", "event_name:"},
		{"GITHUB_ACTOR", "actor:"},
		{"GITHUB_RUN_ID", "run_id:"},
		{"GITHUB_RUN_NUMBER", "run_number:"},
		{"GITHUB_RUN_ATTEMPT", "run_attempt:"},
		{"GITHUB_ENVIRONMENT", "environment:"},
		{"RUNNER_ENVIRONMENT", "runner_environment:"},
		{"RUNNER_OS", "runner_os:"},
		{"RUNNER_ARCH", "runner_arch:"},
	}

	for _, tc := range tests {
		t.Run(tc.envVar, func(t *testing.T) {
			env := make(map[string]string, len(fullEnv))
			for k, v := range fullEnv {
				env[k] = v
			}
			delete(env, tc.envVar)

			plug := newTestPlugin(t, env)
			_, err := plug.Configure(context.Background(), &configv1.ConfigureRequest{})
			if err != nil {
				t.Fatalf("Configure failed: %v", err)
			}

			resp, err := plug.Attest(context.Background(), &workloadattestorv1.AttestRequest{Pid: 1})
			if err != nil {
				t.Fatalf("Attest failed: %v", err)
			}

			for _, v := range resp.SelectorValues {
				if len(v) >= len(tc.selectorPrefix) && v[:len(tc.selectorPrefix)] == tc.selectorPrefix {
					t.Errorf("unexpected selector %q when %s is absent", v, tc.envVar)
				}
			}
		})
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

	selectors := selectorSet(resp)
	if !selectors["env:MY_CUSTOM_VAR:custom-value"] {
		t.Error("expected selector value env:MY_CUSTOM_VAR:custom-value not found")
	}
}
