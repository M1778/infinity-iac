# Infinity-IaC command reference

This document is the maintained product reference for Infinity-IaC 1.0.1.
Compute lifecycle commands use the Oracle Cloud Infrastructure SDK and the
selected OCI CLI profile. Network, sandbox, backup, and wrapper commands remain
reserved until their production layers are implemented.

## Installation and shell completion

### Release installers

Unix users can install the latest tagged release directly from GitHub:

```sh
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh | sh
```

Set `INFINITY_IAC_PREFIX` or pass `--prefix` to choose the installation
directory. The installer supports Linux and macOS on amd64 and arm64, and
verifies the release checksum before writing the binary. Windows users can
download `infinity-iac-setup.exe` from the GitHub Release page.

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

Validates an OCI CLI profile and records the deployment identifiers required to
provision instances. Private credentials stay in the OCI CLI configuration.

```sh
infinity-iac setup oracle \
  --name default --profile DEFAULT --region us-phoenix-1 \
  --compartment COMPARTMENT_OCID --subnet SUBNET_OCID \
  --availability-domain PHX-AD-1 --image IMAGE_OCID \
  --shape VM.Standard.E4.Flex --ssh-public-key "$(cat ~/.ssh/id_ed25519.pub)"
```

### `list acc`

Lists configured access profiles. Use `--output json` for automation.

```sh
infinity-iac list acc --output json
```

### `create server`

Creates one or more OCI instances through the configured production provider.
The selected image OCID is used for the instance OS; `--startup-script` is
encoded as OCI cloud-init user data and runs on first boot.
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

Defaults are Ubuntu latest, 1 vCPU, 1 GB RAM, and 20 GB requested storage.
OCI boot volumes are allocated at the provider minimum of 50 GB when a lower
size is requested.
Networking recipe execution is rejected until a real OCI networking layer is
available.

### `list instances`

Lists owned, non-deleted instances.

```sh
infinity-iac list instances --output json
```

### `list recipes`

Lists the validated built-in recipe catalog. Set `INFINITY_IAC_RECIPES` or use
`--root` to inspect a filesystem catalog during development.

```sh
infinity-iac list recipes
infinity-iac list recipes --root ./recipes --output json
```

### `validate recipe`

Validates one YAML or JSON recipe against `recipe/v1` without executing steps.

```sh
infinity-iac validate recipe --file recipes/n8n/recipe.yaml
```

`recipe list` is an alias for `list recipes`, and `recipe validate` is an alias
for this command. `list capabilities` reports the provider, operating system,
and architecture metadata from the same catalog.

### `instance stop|restart|delete`

Lifecycle operations accept either an instance name or ID.

```sh
infinity-iac instance stop demo
infinity-iac instance restart demo
infinity-iac instance delete demo --yes
```

### `run`

Application and script names are resolved against the validated recipe catalog;
unknown recipes fail before creating anything. Validated recipe steps are sent
to OCI as base64 encoded cloud-init user data and run on first boot. The
`{{port}}` recipe token is replaced with the selected host port.
The command returns after the OCI launch request; cloud-init continues on the
instance.

```sh
infinity-iac run --app n8n --specs auto --port 1080 --startup-script ./start.sh --yes
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

Reports configuration, profile count, release version, and selected provider.

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
