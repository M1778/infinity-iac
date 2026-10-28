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

### app install

Installs a validated third-party recipe into the local app catalog. Recipes
are unsafe and their steps can run as root during run; review the manifest
before confirming. Use --yes for noninteractive operation.

~~~sh
infinity-iac app install owner/repository --yes
infinity-iac app install --external https://example.com/app.yml --yes
infinity-iac app install ./google-app.yml --yes
~~~

GitHub sources are read from one of app.yml, app.yaml, recipe.yml, recipe.yaml,
or recipe.json at the repository root. External sources must use HTTPS. Remote
recipes with upload steps are rejected because their assets cannot be fetched
as part of a single manifest.

### update

Downloads the matching binary and SHA256SUMS from the GitHub Release, verifies
the digest, then atomically replaces the current executable.

~~~sh
infinity-iac update --yes
infinity-iac update --version v1.0.1 --yes
~~~

### `setup oracle`

Loads the exact named OCI CLI profile and performs read-only API calls before
recording the deployment identifiers required to provision instances. It
validates the profile credentials, compartment, subnet, image, availability
domain, and instance-list permission. No OCI resource is created, changed, or
deleted by setup. Private credentials stay in the OCI CLI configuration.

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

### `unlink acc`

Removes an Infinity-IaC access profile from the local configuration. It does not
revoke the OCI CLI profile, delete OCI resources, or contact Oracle. If the
removed profile was the default, the first remaining profile becomes the new
default. Scripts must pass `--yes`; interactive use prompts for confirmation.

```sh
infinity-iac unlink acc default --yes
infinity-iac unlink acc default --yes --output json
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

Lists the validated built-in recipe catalog. In 1.0.1 this catalog contains
exactly `n8n`; the planned 100+ application and script catalog is not shipped
yet. Set `INFINITY_IAC_RECIPES` or use `--root` to inspect a filesystem catalog
during development.

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

### Built-in catalog status

The source of truth for built-ins is `recipes/`, and `recipes/embed.go` embeds
the selected files into release binaries. The current tree has one file:
`recipes/n8n/recipe.yaml`. There are no built-in Docker, Kubernetes, database,
monitoring, security, backup, networking, or general-purpose script recipes in
1.0.1. A filesystem recipe found through `--root` or `INFINITY_IAC_RECIPES` is
an external catalog entry and is not part of the release.

The n8n built-in is an application recipe, not a prebuilt OCI image. OCI boots
the image OCID saved in the access profile; cloud-init then installs `docker.io`
and runs `docker.n8n.io/n8nio/n8n` on host port 5678 by default. Its health
check and resource declarations can be inspected with `list recipes --output
json`.

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

The only built-in invocation in 1.0.1 is `infinity-iac run --app n8n`. No
built-in script recipe exists yet. Both `--app` and `--script` currently
select a recipe by name; they do not enforce the recipe's `kind` field. Use
`validate recipe` with a custom catalog while developing additional recipes.

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

## Configuration and environment

Infinity-IaC stores access profiles as JSON. Set `INFINITY_IAC_CONFIG` to use a
project or CI-specific file; otherwise the file is placed under the operating
system user config directory (for example, `~/.config/infinity-iac/config.json`
on Linux). The file is created with owner-only permissions.

The OCI private key remains in the OCI CLI config referenced by the selected
profile. `setup oracle --config PATH` selects a non-default OCI config file; it does not
copy that file. If omitted, Infinity-IaC uses `OCI_CONFIG_FILE` when set, then
`~/.oci/config` on Unix (or the equivalent home directory on Windows). It reads
the exact profile named by `--profile` and does not fall back to another OCI
profile or fabricate local credentials. `--profile` selects the OCI profile name, while
`--name` names the Infinity-IaC access profile.

Recipe discovery uses this precedence:

1. `--root PATH` on `list recipes` or `list capabilities`.
2. `INFINITY_IAC_RECIPES`.
3. A repository `recipes/` directory when running from a checkout.
4. The catalog embedded in the release binary.

## Output, confirmation, and errors

Commands that support `--output` accept `table` (the default) or `json`. JSON is
indented on stdout; errors are written to stderr and return a non-zero exit
status. Use the JSON form for scripts instead of parsing table columns.

`create server`, `run`, and `apply` are billable operations. `instance delete`
is destructive. Interactive terminals prompt for confirmation; pipes, cron jobs,
and CI runs must pass `--yes`. `--yes` is explicit confirmation and does not
remove provider-side authorization checks.

## OCI lifecycle behavior

The OCI adapter owns only instances tagged `infinity-iac=true`. `list instances`
filters out other OCI instances and terminated instances. It follows OCI list
pagination and looks up the public IP from the first attached VNIC.

A create request uses the profile's compartment, subnet, availability domain,
image OCID, and shape. Flex shapes receive the requested vCPU and RAM values.
The provider always requests a public IP and raises boot-volume requests below
OCI's 50 GB minimum. The returned instance can remain `provisioning` while OCI
finishes placement. `run` and `--startup-script` are cloud-init payloads; their
commands execute on first boot after the launch API call returns.

Stop uses OCI's stop action, restart uses a soft reset, and delete terminates
the instance without preserving its boot volume. Verify the instance name or ID
before deletion.

## Manifest reference

`plan` and `apply` accept `.json`, `.yaml`, and `.yml` files. The supported v1
shape is:

```yaml
api_version: v1
project: optional-label
servers:
  - name: demo
    provider: oracle
    access: default
    region: us-phoenix-1
    os: ubuntu-latest
    cpu: 2
    ram_gb: 2
    storage_gb: 50
    port: 8080
    startup_script: ./bootstrap.sh
```

Missing OS, CPU, RAM, and storage values default to `ubuntu-latest`, 1 vCPU,
1 GB, and 20 GB. Provider, access, and region default from the selected access
profile when the CLI loads the manifest. The plan hash covers the normalized
actions, not the plan timestamp, so `apply` can safely recompute it after review.
`apply` currently executes create actions; it does not modify or destroy an
existing instance that is absent from the manifest.

## Troubleshooting

- **No access profile configured:** run `setup oracle`, or point
  `INFINITY_IAC_CONFIG` at the file that contains the profile.
- **OCI configuration is incomplete:** provide compartment, subnet, availability
  domain, image, shape, region, and an `oci-profile:NAME` credential source.
- **OCI access validation fails:** run the OCI CLI with the same `--profile`,
  `--config`, and region, then verify that the profile can list instances in
  the compartment.
- **Recipe is not found:** run `list recipes` and check the catalog precedence
  above. A custom recipe must pass `validate recipe` first.
- **Instance is running but the app is unavailable:** wait for cloud-init,
  inspect `/var/log/cloud-init-output.log` on the instance, and verify that the
  subnet's security rules allow the recipe port. `resources.ingress` is metadata
  in 1.0.1 and does not edit OCI network rules.
