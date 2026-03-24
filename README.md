# spire-github-actions-plugin

![Test](https://github.com/aizu-hiroki/spire-github-actions-plugin/actions/workflows/test.yml/badge.svg)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](go.mod)

> **⚠️ EXPERIMENTAL — USE AT YOUR OWN RISK**
>
> This project is experimental and provided "as-is" without any warranty or
> guarantee of any kind. It has not been audited for security and is not
> intended for production use. The authors and contributors accept no
> responsibility or liability for any damages, data loss, security incidents,
> or other consequences arising from the use of this software. **You use this
> software entirely at your own risk.**

> This is an **unofficial** community plugin and is not affiliated with or
> endorsed by SPIFFE, SPIRE, the CNCF, or GitHub.

---

A [SPIRE](https://github.com/spiffe/spire) plugin that enables GitHub Actions
workflows to authenticate using GitHub's OIDC tokens for Node Attestation.

## How it works

```
GitHub Actions runner
  └── SPIRE agent
        └── sends GitHub Actions OIDC token to SPIRE server
              └── server validates JWT signature via GitHub's JWKS
                    └── issues SPIFFE agent ID + node-level selectors
                          └── workloads on the runner obtain SVIDs
                                via the Workload API
```

The OIDC token is cryptographically signed by GitHub and verified against
GitHub's public JWKS endpoint. No long-lived credentials are required.

## Plugins

| Binary | Type | Description |
|--------|------|-------------|
| `spire-plugin-github-actions-agent` | Node Attestor (agent-side) | Fetches a GitHub Actions OIDC token and sends it to the SPIRE server |
| `spire-plugin-github-actions-server` | Node Attestor (server-side) | Validates the JWT using GitHub's JWKS and returns a SPIFFE agent ID and node-level selectors |

## Requirements

- Go 1.26+
- SPIRE v1.x
- Linux

## Build

```bash
go mod tidy
make build
# Binaries are written to bin/
```

## Configuration

### SPIRE Agent (`spire-agent.conf`)

```hcl
NodeAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-agent"
  plugin_data {
    # Must match the server plugin's audience setting.
    # Use a value that uniquely identifies your SPIRE server,
    # e.g. the trust domain URI.
    audience = "spiffe://example.org"
  }
}
```

### SPIRE Server (`spire-server.conf`)

At least one of `allowed_repository_owners` or `allowed_repositories` must be
configured. Omitting both is rejected at startup to prevent unintentionally
allowing attestation from any GitHub repository.

```hcl
NodeAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-server"
  plugin_data {
    # Required: restrict attestation to specific GitHub organisation/user names.
    allowed_repository_owners = ["your-org"]

    # Optional: further restrict to specific repositories (owner/repo format).
    # allowed_repositories = ["your-org/your-repo"]

    # Required: must match the agent plugin's audience setting. Must not be empty.
    audience = "spiffe://example.org"

    # Optional: override OIDC issuer (for GitHub Enterprise Server).
    # oidc_issuer_url = "https://token.actions.githubusercontent.com"
  }
}
```

### GitHub Actions Workflow

The workflow must have `id-token: write` permission to obtain an OIDC token:

```yaml
permissions:
  id-token: write
  contents: read
```

## Generated SPIFFE Agent ID

After successful attestation, the SPIRE agent is identified by:

```
spiffe://<trust-domain>/spire/agent/github_actions/<owner>/<repo>
```

Example:

```
spiffe://example.org/spire/agent/github_actions/my-org/my-repo
```

## Selectors

These selectors are derived from the GitHub Actions OIDC JWT and are
cryptographically verified via GitHub's JWKS. SPIRE stores them as
node-level selectors for the attested agent.

| Selector value | Source |
|----------------|--------|
| `repository:<owner>/<repo>` | JWT `repository` claim |
| `repository_owner:<owner>` | JWT `repository_owner` claim |
| `repository_id:<id>` | JWT `repository_id` claim |
| `repository_owner_id:<id>` | JWT `repository_owner_id` claim |
| `repository_visibility:<visibility>` | JWT `repository_visibility` claim |
| `workflow:<name>` | JWT `workflow` claim |
| `workflow_ref:<ref>` | JWT `workflow_ref` claim |
| `job_workflow_ref:<ref>` | JWT `job_workflow_ref` claim |
| `ref:<ref>` | JWT `ref` claim |
| `ref_type:<type>` | JWT `ref_type` claim |
| `branch:<name>` | derived from `ref` when `ref_type` is `branch` (e.g. `refs/heads/main` → `branch:main`) |
| `sha:<sha>` | JWT `sha` claim |
| `head_ref:<ref>` | JWT `head_ref` claim (pull requests only) |
| `base_ref:<ref>` | JWT `base_ref` claim (pull requests only) |
| `event_name:<event>` | JWT `event_name` claim |
| `actor:<user>` | JWT `actor` claim |
| `actor_id:<id>` | JWT `actor_id` claim |
| `run_id:<id>` | JWT `run_id` claim |
| `run_number:<n>` | JWT `run_number` claim |
| `run_attempt:<n>` | JWT `run_attempt` claim |
| `environment:<name>` | JWT `environment` claim (deployment jobs only) |
| `runner_environment:<type>` | JWT `runner_environment` claim |

## Issuing SVIDs to Workloads

### Pattern 1: Repository-scoped (simple)

Use the agent's SPIFFE ID as `parentID` to restrict SVID issuance to a
specific repository. Combined with the built-in `unix` workload attestor:

```bash
spire-server entry create \
  -spiffeID "spiffe://example.org/deploy/production" \
  -parentID "spiffe://example.org/spire/agent/github_actions/my-org/my-repo" \
  -selector "unix:uid:1001"
```

### Pattern 2: Branch-scoped (node entry chaining)

Use a node entry to assign an alias SPIFFE ID to agents matching a specific
branch, then issue workload SVIDs from that alias. This allows different
identities for jobs running on different branches.

```bash
# Step 1: assign an alias to agents attested from the main branch
spire-server entry create \
  -spiffeID "spiffe://example.org/ci/branch/main" \
  -parentID  "spiffe://example.org/spire/agent/github_actions/my-org/my-repo" \
  -selector  "github_actions:branch:main" \
  -node

# Step 2: issue a workload SVID scoped to that branch
spire-server entry create \
  -spiffeID "spiffe://example.org/deploy/production" \
  -parentID  "spiffe://example.org/ci/branch/main" \
  -selector  "unix:uid:1001"
```

Only a runner attested from `my-org/my-repo` on the `main` branch, running
a process as UID 1001, will receive the `deploy/production` SVID.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
