package github

import "fmt"

// BuildSelectors generates SPIRE selector values from GitHub Actions OIDC claims.
// Each value is in "key:value" format; SPIRE prepends the plugin name automatically.
// Only non-empty claim values are included.
func BuildSelectors(claims *Claims) []string {
	var selectors []string

	add := func(key, value string) {
		if value != "" {
			selectors = append(selectors, fmt.Sprintf("%s:%s", key, value))
		}
	}

	// Core identity selectors.
	add("repository", claims.Repository)
	add("repository_owner", claims.RepositoryOwner)
	add("repository_id", claims.RepositoryID)
	add("repository_owner_id", claims.RepositoryOwnerID)
	add("repository_visibility", claims.RepositoryVisibility)

	// Workflow selectors.
	add("workflow", claims.Workflow)
	add("workflow_ref", claims.WorkflowRef)
	add("job_workflow_ref", claims.JobWorkflowRef)

	// Git ref selectors.
	add("ref", claims.Ref)
	add("ref_type", claims.RefType)
	add("sha", claims.SHA)
	add("head_ref", claims.HeadRef)
	add("base_ref", claims.BaseRef)

	// Trigger selectors.
	add("event_name", claims.EventName)
	add("actor", claims.Actor)
	add("actor_id", claims.ActorID)

	// Run selectors.
	add("run_id", claims.RunID)
	add("run_number", claims.RunNumber)
	add("run_attempt", claims.RunAttempt)

	// Environment (only present for deployment jobs).
	add("environment", claims.Environment)

	// Runner selectors.
	add("runner_environment", claims.RunnerEnvironment)

	return selectors
}

// AgentID generates the SPIFFE agent ID from the trust domain and repository.
// Format: spiffe://<trust-domain>/spire/agent/github_actions/<owner>/<repo>
func AgentID(trustDomain, repository string) string {
	return fmt.Sprintf("spiffe://%s/spire/agent/github_actions/%s", trustDomain, repository)
}
