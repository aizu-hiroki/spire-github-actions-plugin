# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-03-22

### Added
- Node Attestor (agent-side): fetches GitHub Actions OIDC token and sends it to the SPIRE server
- Node Attestor (server-side): validates JWT using GitHub's JWKS, returns SPIFFE agent ID and selectors
- Workload Attestor: reads `/proc/<pid>/environ` and returns GitHub Actions context as selectors
- Support for `allowed_repository_owners` to restrict attestation by GitHub owner
- Configurable `audience` claim for OIDC token validation
- Configurable `extra_env_vars` for emitting additional environment variables as selectors
- Unit tests for all plugins including NG cases (invalid signature, expired token, wrong issuer/audience)
- E2E test using real GitHub Actions OIDC token against a local SPIRE server
