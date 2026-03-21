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

[日本語](#日本語)

---

A [SPIRE](https://github.com/spiffe/spire) plugin that enables GitHub Actions
workflows to authenticate using GitHub's OIDC tokens for both Node Attestation
and Workload Attestation.

## Plugins

| Binary | Type | Description |
|--------|------|-------------|
| `spire-plugin-github-actions-agent` | Node Attestor (agent-side) | Fetches a GitHub Actions OIDC token and sends it to the SPIRE server |
| `spire-plugin-github-actions-server` | Node Attestor (server-side) | Validates the JWT using GitHub's JWKS, returns a SPIFFE ID and selectors |
| `spire-plugin-github-actions-workload` | Workload Attestor | Reads `/proc/<pid>/environ` on Linux and returns GitHub Actions context as selectors |

## Requirements

- Go 1.21+
- SPIRE v1.x
- Linux (workload attestor requires `/proc` filesystem)

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

WorkloadAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-workload"
  plugin_data {
    # Optional: emit additional env vars as selectors.
    # extra_env_vars = ["MY_CUSTOM_VAR"]
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

## Selectors

The following selectors are produced by both the node attestor and workload attestor:

| Selector | Source |
|----------|--------|
| `github_actions:repository:<owner>/<repo>` | `GITHUB_REPOSITORY` |
| `github_actions:repository_owner:<owner>` | `GITHUB_REPOSITORY_OWNER` |
| `github_actions:workflow:<name>` | `GITHUB_WORKFLOW` |
| `github_actions:workflow_ref:<ref>` | `GITHUB_WORKFLOW_REF` |
| `github_actions:job:<id>` | `GITHUB_JOB` |
| `github_actions:ref:<ref>` | `GITHUB_REF` |
| `github_actions:ref_type:<type>` | `GITHUB_REF_TYPE` |
| `github_actions:sha:<sha>` | `GITHUB_SHA` |
| `github_actions:head_ref:<ref>` | `GITHUB_HEAD_REF` (pull requests) |
| `github_actions:base_ref:<ref>` | `GITHUB_BASE_REF` (pull requests) |
| `github_actions:event_name:<event>` | `GITHUB_EVENT_NAME` |
| `github_actions:actor:<user>` | `GITHUB_ACTOR` |
| `github_actions:run_id:<id>` | `GITHUB_RUN_ID` |
| `github_actions:run_number:<n>` | `GITHUB_RUN_NUMBER` |
| `github_actions:run_attempt:<n>` | `GITHUB_RUN_ATTEMPT` |
| `github_actions:environment:<name>` | `GITHUB_ENVIRONMENT` (deployment jobs) |
| `github_actions:runner_environment:<type>` | `RUNNER_ENVIRONMENT` |
| `github_actions:runner_os:<os>` | `RUNNER_OS` |
| `github_actions:runner_arch:<arch>` | `RUNNER_ARCH` |

## Generated SPIFFE Agent ID

```
spiffe://<trust-domain>/spire/agent/github_actions/<owner>/<repo>
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

## 日本語

GitHub Actions の OIDC トークンを使った [SPIRE](https://github.com/spiffe/spire) 向けの
Node Attestation / Workload Attestation プラグインです。

### プラグイン構成

| バイナリ | 種別 | 説明 |
|---------|------|------|
| `spire-plugin-github-actions-agent` | Node Attestor (agent) | GitHub Actions OIDC トークンを取得してサーバーへ送信 |
| `spire-plugin-github-actions-server` | Node Attestor (server) | JWT を検証し SPIFFE ID とセレクターを返す |
| `spire-plugin-github-actions-workload` | Workload Attestor | `/proc/<pid>/environ` から GitHub Actions コンテキストを読み取りセレクターを返す |

### ビルド

```bash
go mod tidy
make build
# bin/ 以下にバイナリが生成されます
```

### GitHub Actions ワークフロー設定

```yaml
permissions:
  id-token: write   # OIDC トークンの取得に必要
  contents: read
```

### 免責事項

本プロジェクトは非公式のコミュニティプラグインであり、SPIFFE、SPIRE、CNCF、および GitHub とは一切関係がなく、これらによる承認を受けたものでもありません。

本ソフトウェアは実験的なものであり、現状のまま（"as-is"）提供されます。
明示または黙示を問わず、いかなる保証も行いません。
本ソフトウェアの使用によって生じたいかなる損害・損失・セキュリティ上の問題についても、
作者および貢献者は一切の責任を負いません。
**本ソフトウェアの利用はすべて利用者自身の責任において行ってください。**
