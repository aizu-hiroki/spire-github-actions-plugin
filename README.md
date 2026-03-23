# spire-github-actions-plugin

![Test](https://github.com/aizu-hiroki/spire-github-actions-plugin/actions/workflows/test.yml/badge.svg)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](go.mod)

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
                    └── issues SPIFFE agent ID to the runner
                          └── workloads on the runner obtain SVIDs
                                via the Workload API
```

The OIDC token is cryptographically signed by GitHub and verified against
GitHub's public JWKS endpoint. No long-lived credentials are required.

## Plugins

| Binary | Type | Description |
|--------|------|-------------|
| `spire-plugin-github-actions-agent` | Node Attestor (agent-side) | Fetches a GitHub Actions OIDC token and sends it to the SPIRE server |
| `spire-plugin-github-actions-server` | Node Attestor (server-side) | Validates the JWT using GitHub's JWKS and returns a SPIFFE agent ID |

## Requirements

- Go 1.21+
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

```hcl
NodeAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-server"
  plugin_data {
    # Restrict attestation to specific GitHub organisation/user names.
    allowed_repository_owners = ["your-org"]

    # Must match the agent plugin's audience setting.
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

## Issuing SVIDs to Workloads

Use the agent's SPIFFE ID as the `parentID` when creating registration entries.
Combined with a workload attestor (e.g., the built-in `unix` attestor), you can
issue SVIDs to specific processes running on the GitHub Actions runner.

```bash
spire-server entry create \
  -spiffeID "spiffe://example.org/deploy/production" \
  -parentID "spiffe://example.org/spire/agent/github_actions/my-org/my-repo" \
  -selector "unix:uid:1001"
```

This restricts SVID issuance to processes running as UID 1001 on a runner
attested from the `my-org/my-repo` repository.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
