# Infinity-IaC command reference

This document is the maintained product reference. The executable currently
implements the local provider slice described in **Available commands**. The
later provider, network, sandbox, backup, and wrapper commands are reserved
for the corresponding implementation layers.

## Installation and shell completion

Build the binary, place it on `PATH`, then generate the completion script for
the shell in use:

```sh
go build -trimpath -o bin/infinity-iac ./cmd/infinity-iac
```

```sh
# Bash
source <(infinity-iac completion bash)

# Zsh
source <(infinity-iac completion zsh)

# Fish
infinity-iac completion fish | source
```

```powershell
# PowerShell
infinity-iac completion powershell | Out-String | Invoke-Expression
```

The generated adapters call the same shell-neutral completion protocol used by
the CLI. Prefixes such as `infinity-iac cr<Tab>` resolve to `create`, and
nested prefixes such as `infinity-iac create s<Tab>` resolve to `server` and
`sandbox`.

## Available commands

### `setup oracle`

Records an Oracle access profile without copying private credentials into
Infinity-IaC configuration.

```sh
infinity-iac setup oracle \
  --name default \
  --profile DEFAULT \
  --account TENANCY_ID \
  --region us-phoenix-1
```

### `list acc`

Lists configured access profiles. Use `--output json` for automation.

```sh
infinity-iac list acc --output json
```

### `create server`

Creates one or more instances through the configured local provider backend.
Billable or destructive operations require `--yes` in noninteractive use.

```sh
infinity-iac create server \
  --name demo \
  --os ubuntu-latest \
  --cpu 2 \
  --ram 2GB \
  --storage-size 40GB \
  --provider oracle \
  --amount 2 \
  --yes
```

Defaults are Ubuntu latest, 1 vCPU, 1 GB RAM, 20 GB storage, and the `iauto`
networking recipe marker.

### `list instances`

Lists owned, non-deleted instances.

```sh
infinity-iac list instances --output json
```

### `instance stop|restart|delete`

Lifecycle operations accept either an instance name or ID.

```sh
infinity-iac instance stop demo
infinity-iac instance restart demo
infinity-iac instance delete demo --yes
```

### `run`

The current slice accepts `--app` or `--script` and provisions a shared server
record. Application recipe execution and readiness checks are planned for the
recipe engine.

```sh
infinity-iac run --app n8n --specs auto --port 1080 --yes
infinity-iac run --script windows-server --specs auto --yes
```

### `plan` and `apply`

Plans accept JSON or YAML manifests. Review the plan JSON, then pass its hash
to `apply`; a changed manifest is rejected.

```yaml
api_version: v1
servers:
  - name: demo
    provider: oracle
    os: ubuntu-latest
    cpu: 2
    ram_gb: 2
    storage_gb: 40
```

```sh
infinity-iac plan --file infrastructure.yaml --output json > plan.json
infinity-iac apply --file infrastructure.yaml \
  --hash "$(jq -r .hash plan.json)" --yes
```

### `doctor`

Reports configuration, local state, and provider backend information.

```sh
infinity-iac doctor --output json
```

### `install skills`

Installs a minimal versioned agent-harness instruction file. `--dry-run` shows
the target path without writing it.

```sh
infinity-iac install skills --dry-run
```

## Reserved command areas

The product design reserves these namespaces for future typed resource layers:

- `network`: networks, subnets, routes, gateways, security rules, public IPs,
  and DNS attachments.
- `sandbox`: expiring environments with limits and optional backups.
- `wrap`: temporary infrastructure around a command with remote expiry.
- `import`: explicit adoption of existing provider resources.

Reserved commands should fail clearly until their owning layer is implemented;
they must not silently emulate a real cloud operation.
