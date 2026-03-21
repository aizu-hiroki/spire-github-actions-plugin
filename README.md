# spire-github-actions-plugin

> **⚠️ EXPERIMENTAL — USE AT YOUR OWN RISK**
>
> This project is experimental and provided "as-is" without any warranty or
> guarantee of any kind.  It has not been audited for security and is not
> intended for production use.  The authors and contributors accept no
> responsibility or liability for any damages, data loss, security incidents,
> or other consequences arising from the use of this software.  **You use this
> software entirely at your own risk.**

---

GitHub Actions OIDC を使った [SPIRE](https://github.com/spiffe/spire) 向けの
Node Attestation / Workload Attestation プラグインです。

## プラグイン構成

| バイナリ | 種別 | 説明 |
|---|---|---|
| `spire-plugin-github-actions-agent` | Node Attestor (agent) | GitHub Actions OIDC トークンを取得してサーバーへ送信 |
| `spire-plugin-github-actions-server` | Node Attestor (server) | JWT を検証し SPIFFE ID とセレクターを返す |
| `spire-plugin-github-actions-workload` | Workload Attestor | `/proc/<pid>/environ` から GitHub Actions コンテキストを読み取りセレクターを返す |

## ビルド

```bash
go version  # 1.21 以上が必要
go mod tidy
make build
# bin/ 以下にバイナリが生成されます
```

## 設定例

### SPIRE Agent (`spire-agent.conf`)

```hcl
NodeAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-agent"
  plugin_data {
    audience = "spire-server"
  }
}

WorkloadAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-workload"
  plugin_data {}
}
```

### SPIRE Server (`spire-server.conf`)

```hcl
NodeAttestor "github_actions" {
  plugin_cmd  = "/usr/local/bin/spire-plugin-github-actions-server"
  plugin_data {
    allowed_repository_owners = ["your-org"]
    audience                  = "spire-server"
  }
}
```

### GitHub Actions ワークフロー

```yaml
permissions:
  id-token: write   # OIDC トークンの取得に必要
  contents: read
```

## 生成されるセレクター例

```
github-actions:repository:your-org/your-repo
github-actions:repository_owner:your-org
github-actions:workflow:CI
github-actions:ref:refs/heads/main
github-actions:ref_type:branch
github-actions:event_name:push
github-actions:actor:octocat
github-actions:environment:production
github-actions:runner_environment:github-hosted
```

## 免責事項

本ソフトウェアは実験的なものであり、現状のまま（"as-is"）提供されます。
明示または黙示を問わず、いかなる保証も行いません。
本ソフトウェアの使用によって生じたいかなる損害・損失・セキュリティ上の問題についても、
作者および貢献者は一切の責任を負いません。
**本ソフトウェアの利用はすべて利用者自身の責任において行ってください。**

## ライセンス

Apache License 2.0 — 詳細は [LICENSE](LICENSE) を参照してください。
