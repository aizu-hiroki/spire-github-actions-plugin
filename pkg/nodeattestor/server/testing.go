package server

import githuboidc "github.com/aizu-hiroki/spire-github-actions-plugin/internal/github"

// SetValidatorForTest replaces the token validator with a test instance.
// This is exported for testing only and must not be called in production code.
func (p *Plugin) SetValidatorForTest(v *githuboidc.TokenValidator) {
	p.mu.Lock()
	p.validator = v
	p.mu.Unlock()
}
