# Changelog

All notable changes to this project will be documented in this file.

## [0.2.0] - 2026-03-24

### Changed
- Removed Workload Attestor based on community feedback (credit: Kevin Fox, SPIFFE community).
  Environment variables read from `/proc/<pid>/environ` cannot be cryptographically verified
  and should not be used for security-critical attestation. All GitHub Actions context is
  validated exclusively at the Node Attestor level via the OIDC JWT.
- README rewritten to focus on `parentID`-based SVID issuance with a concrete usage example.
- Simplified internal `BuildSelectors` API to return `[]string` directly.

## [0.1.0] - 2026-03-22

### Added
- Node Attestor (agent-side): fetches GitHub Actions OIDC token and sends it to the SPIRE server
- Node Attestor (server-side): validates JWT using GitHub's JWKS, returns SPIFFE agent ID and selectors
- Support for `allowed_repository_owners` to restrict attestation by GitHub owner
- Configurable `audience` claim for OIDC token validation
- Unit tests including NG cases (invalid signature, expired token, wrong issuer/audience)
- E2E test using real GitHub Actions OIDC token against a local SPIRE server
