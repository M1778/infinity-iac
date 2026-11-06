# Recipe format

Recipes are YAML or JSON documents with `api_version: recipe/v1`. They are
validated before they appear in `list recipes` and before `run` creates an OCI
instance. A recipe must contain at least one step and must have a unique name
within the selected catalog.

## Catalog selection

Source-built binaries embed 136 definitions: 131 application entries and five
scripts. The published v1.0.1 release predates this expansion. Incomplete entries
are templates, not deployable applications. The CLI uses its embedded catalog
and installed recipes; it does not implicitly execute a current-directory
`recipes/` folder. Set `INFINITY_IAC_RECIPES` or use `--root` for inspection:

```sh
INFINITY_IAC_RECIPES="$PWD/my-recipes" infinity-iac list recipes
infinity-iac list recipes --root ./my-recipes --output json
infinity-iac validate recipe --file ./my-recipes/demo/recipe.yaml
```

`run` uses the same catalog resolution rules. Validate a custom catalog before
using it in automation.

## Schema

```yaml
api_version: recipe/v1
name: demo
version: 1.0.0
kind: application                 # application or script
status: experimental              # experimental, template, or verified
description: Example service
supports:
  providers: [oracle]
  os: [ubuntu-latest, ubuntu-24.04]
  architectures: [amd64, arm64]
inputs:
  port:
    type: integer                  # string, integer, or boolean
    description: Service port
    default: 8080
    min: 1
    max: 65535
resources:
  specs: ibase2                    # catalog metadata; run uses cpu/ram/storage
  cpu: 2
  ram_gb: 2
  storage_gb: 40
  ingress:
    - port: 8080
      protocol: tcp                # tcp or udp
      source: loopback
steps:
  - id: install-curl
    type: package                  # package, command, or upload
    package: curl
  - id: install-service
    type: command
    command: /usr/local/bin/demo --listen 127.0.0.1:{{port}}
    timeout_seconds: 180
  - id: write-config
    type: upload
    source: ./demo.conf             # read on the machine running the CLI
    destination: /etc/demo/demo.conf
    timeout_seconds: 30
health:
  - type: http                     # http or command
    port: 8080
    path: /
    timeout_seconds: 120
outputs:
  url:
    type: string
    value: http://127.0.0.1:{{port}}
```

### Metadata and resources

`supports` documents the provider, OS labels, and architectures a recipe is
intended for. The current OCI runner selects the image and shape from the
configured access profile, then uses `resources.cpu`, `resources.ram_gb`, and
`resources.storage_gb` for the instance request. Missing or non-positive
resource values fall back to 1 vCPU, 1 GB RAM, and 20 GB storage. OCI may apply
its own minimums, including the 50 GB boot-volume minimum.

`resources.ingress` is metadata and does not create OCI security-list or network
security-group rules. Built-in services bind to loopback; use SSH tunnels or a
deliberately configured authenticated proxy. Restrict SSH ingress and provide
outbound package/image access through your existing VCN.

### Steps

- `package` steps are collected into one apt installation with lock timeouts.
  `docker.io` also enables the Docker service.
- `command` writes the command into the generated cloud-init script.
- `upload` reads `source` locally, base64-encodes it, and writes it to the
  absolute `destination` on first boot.

The `{{port}}` token is replaced with the selected port. `--port` overrides the
recipe's `port` input default. Recipe commands run as root through cloud-init;
quote or validate values before placing them in a command.

The schema accepts other typed inputs for metadata validation; the runner only
resolves the `port` default/token. Command steps with `timeout_seconds` are
wrapped in a bounded Bash invocation. Commands run with `set -euo pipefail` and
first-boot recipes serialize through a lock. Upload/package timeout fields are
metadata; packages use their own apt lock timeout.

Health checks run in bounded retry loops. HTTP probes have connect/request
timeouts and use localhost. Command probes run through a short per-attempt
timeout. Health failure makes bootstrap fail. Success writes
`/var/lib/infinity-iac/ready/<name>` after all steps/probes; a prior marker is
cleared before a rerun. A successful OCI launch is not successful application
readiness, so inspect cloud-init logs and representative app behavior.

## Running a recipe

```sh
infinity-iac run --app demo --port 8080 --name demo --yes
infinity-iac run --app n8n --startup-script ./extra.sh --output json --yes
```

`--specs` currently accepts only `auto`; the recipe resources are used. An
unknown recipe fails before an OCI create request. `--startup-script` is read
locally and appended to the generated cloud-init script. Exactly one of `--app`, `--script`, or `--recipe` selects a name; these flags do
not filter its kind. All `run` invocations provision a new VM. `recipe render`
provides an offline preview. Unknown names and templates fail before creation.

## Built-in example

The release catalog contains `n8n`, which installs Docker and starts n8n on its
configured port:

```sh
infinity-iac list recipes
infinity-iac run --app n8n --port 5678 --yes
```

The recipe's declared URL and secret outputs are catalog metadata in 1.0.1; the
CLI does not yet retrieve generated values from the running instance.

## Catalog expansion status

The built-in recipes live under `recipes/<name>/recipe.yaml`; adding a recipe
to a local directory does not add it to release binaries. A built-in must be
reviewed, included by `recipes/embed.go`, and covered by catalog validation and
execution tests. Each recipe has
an owner, supported OCI image and architecture, resource sizing, ports and
network requirements, health checks, and a documented rollback or cleanup
path.

See [the built-in catalog](builtin-catalog.md) for the exact release inventory
and the complete first-boot behavior of `n8n`.

## Readiness and persistence

All built-ins carry readiness status. `experimental` means available for
controlled evaluation, not production certified. `template` requires a
nonempty `requirements` list and is blocked from rendering/deployment.
`verified` should be used only with documented runtime evidence for the tested
version, platform, and application scope. Schema checks alone are not evidence.

Stateful demos use named volumes so an owned container can be recreated
without losing its data. Reruns refuse containers not labeled for the recipe.
Use separate backups before deleting a VM; termination with its boot volume
also removes local Docker volumes. Generated bootstrap secrets belong in
owner-only files on the remote instance and must not be presented as retrieved
CLI outputs. Onboarding-only demos need explicit administrator/TLS settings
before external publication.
