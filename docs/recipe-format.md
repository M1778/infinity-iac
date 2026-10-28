# Recipe format

Recipes are YAML or JSON documents with `api_version: recipe/v1`. They are
validated before they appear in `list recipes` and before `run` creates an OCI
instance. A recipe must contain at least one step and must have a unique name
within the selected catalog.

## Catalog selection

Release binaries include the validated built-in catalog. When running from a
checkout, the CLI uses a `recipes/` directory if it exists. Set
`INFINITY_IAC_RECIPES` or pass `--root` to inspect another catalog:

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
      source: public
steps:
  - id: install-curl
    type: package                  # package, command, or upload
    package: curl
  - id: install-service
    type: command
    command: /usr/local/bin/demo --port {{port}}
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
    value: http://${instance.public_ip}:8080
  token:
    type: secret
    value: generated
    sensitive: true
```

### Metadata and resources

`supports` documents the provider, OS labels, and architectures a recipe is
intended for. The current OCI runner selects the image and shape from the
configured access profile, then uses `resources.cpu`, `resources.ram_gb`, and
`resources.storage_gb` for the instance request. Missing or non-positive
resource values fall back to 1 vCPU, 1 GB RAM, and 20 GB storage. OCI may apply
its own minimums, including the 50 GB boot-volume minimum.

`resources.ingress` is validated and listed as recipe metadata. It does not yet
create OCI security-list or network-security-group rules, so open the required
port in the subnet separately.

### Steps

- `package` runs `apt-get update` and installs the named package on the Ubuntu
  instance.
- `command` writes the command into the generated cloud-init script.
- `upload` reads `source` locally, base64-encodes it, and writes it to the
  absolute `destination` on first boot.

The `{{port}}` token is replaced with the selected port. `--port` overrides the
recipe's `port` input default. Recipe commands run as root through cloud-init;
quote or validate values before placing them in a command.

The schema accepts other typed inputs for catalog validation, but 1.0.1 only
resolves the `port` default and token. Step `timeout_seconds` values are
validated and retained in the recipe; the generated script does not yet wrap
each individual step in a timeout.

Health checks are appended to the same first-boot script. HTTP checks use
localhost and retry with `curl`; command checks run directly. A successful OCI
launch does not mean that cloud-init has finished, so inspect the instance and
its cloud-init logs when a service is not ready.

## Running a recipe

```sh
infinity-iac run --app demo --port 8080 --name demo --yes
infinity-iac run --script n8n --startup-script ./extra.sh --output json --yes
```

`--specs` currently accepts only `auto`; the recipe resources are used. An
unknown recipe fails before an OCI create request. `--startup-script` is read
locally and appended to the generated cloud-init script.

## Built-in example

The release catalog contains `n8n`, which installs Docker and starts n8n on its
configured port:

```sh
infinity-iac list recipes
infinity-iac run --app n8n --port 5678 --yes
```

The recipe's declared URL and secret outputs are catalog metadata in 1.0.1; the
CLI does not yet retrieve generated values from the running instance.
