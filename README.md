# Infinity-IaC 1.0.1

Infinity-IaC is a Go CLI for Oracle Cloud Infrastructure (OCI) compute
lifecycle workflows. It creates, lists, stops, restarts, and terminates OCI
instances, runs validated application recipes through cloud-init, and supports
reviewable JSON/YAML plans with a hash-bound apply step.

The CLI uses the OCI Go SDK and the standard OCI CLI credential file. Private
keys are never copied into the Infinity-IaC configuration file. The release
binary embeds the built-in recipe catalog and emits table output for humans or
JSON for automation.

## Install

### Unix

The installer supports Linux and macOS on amd64 and arm64. It downloads the
latest GitHub Release asset and verifies `SHA256SUMS` before installing:

```sh
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh | sh
```

Pin a release or choose a destination when needed:

```sh
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh \
  | sh -s -- --version v1.0.1 --prefix "$HOME/.local/bin"
```

The same values can be supplied with `INFINITY_IAC_VERSION`,
`INFINITY_IAC_PREFIX`, and `INFINITY_IAC_REPOSITORY`. Add the selected prefix to
`PATH` if the installer reports that it is not already present.

### Windows

Download `infinity-iac-setup.exe` from the [GitHub Release](https://github.com/M1778/infinity-iac/releases).
The per-user installer places `infinity-iac.exe` under the user programs
directory and offers to add that directory to the user `PATH`. Restart the
shell after changing `PATH`.

### Build from source

Go 1.23 or newer is required:

```sh
go build -trimpath -o bin/infinity-iac ./cmd/infinity-iac
```

Put `bin/infinity-iac` on `PATH`, or use that path in the examples below.

## First OCI deployment

You need an OCI tenancy, a compartment, a subnet with a public-IP policy, an
availability domain, and an image OCID in the selected region. Install and
configure the [OCI CLI](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm)
first (normally with `oci setup config`). Confirm that the selected profile can
read the compartment and subnet before running the setup command.

Choose a private config location for project-local automation, or omit
`INFINITY_IAC_CONFIG` to use the platform user config directory:

```sh
export INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json"
infinity-iac setup oracle \
  --name default \
  --profile DEFAULT \
  --region us-phoenix-1 \
  --compartment ocid1.compartment.oc1..example \
  --subnet ocid1.subnet.oc1..example \
  --availability-domain PHX-AD-1 \
  --image ocid1.image.oc1..example \
  --shape VM.Standard.E4.Flex \
  --ssh-public-key "$(cat ~/.ssh/id_ed25519.pub)"
```

`setup oracle` loads the exact OCI profile and performs live, read-only API
checks for the credentials, compartment, subnet, image, availability domain,
image/shape compatibility, and instance-list permission before saving the
profile. It stores identifiers and the OCI profile name, but not the private
key. Setup creates no OCI resources. The first profile becomes the default;
later profiles can be selected with `--access` on create commands.

Create and inspect an instance:

```sh
infinity-iac create server --name demo --cpu 2 --ram 2GB \
  --storage-size 50GB --provider oracle --yes
infinity-iac list instances
infinity-iac list instances --output json
```

`--os` is a label used in the local instance record. The image actually booted
is the image OCID saved by `setup oracle`. OCI boot volumes have a 50 GB
provider minimum, so a smaller requested size is raised to 50 GB.

## Built-in app and script catalog

The 1.0.1 release does **not** contain the planned 100+ popular applications
and scripts. Its embedded catalog currently contains exactly one recipe:

| Name | Kind | What it does | Default resources |
| --- | --- | --- | --- |
| `n8n` | application | Installs Docker and starts the n8n workflow automation service | 2 vCPU, 2 GB RAM, 35 GB requested storage |

The n8n recipe uses the OCI image OCID from your access profile as its Ubuntu
base image, installs the Ubuntu `docker.io` package, and launches
`docker.n8n.io/n8nio/n8n` with `--restart unless-stopped`. Its default host
port is 5678, its declared HTTP health check targets `/`, and its declared
URL output is metadata; 1.0.1 does not yet fetch generated application output
values from the instance.

Inspect the catalog shipped by the binary:

```sh
infinity-iac list recipes
infinity-iac list capabilities --output json
```

`run --app` and `run --script` both resolve names from this same validated
recipe catalog. `run --script` is not yet a separate library of shell scripts,
and a name that is not listed fails before an OCI instance is created. The
catalog code supports adding more versioned YAML/JSON recipes, but those files
must be added, validated, embedded, and tested before they become built-ins.

The 100+ catalog is therefore a development target, not a feature already
present in 1.0.1. Track its implementation in the catalog roadmap in
`TODO.md` and treat each recipe as production code: it needs a supported OCI
image/architecture, idempotent bootstrap steps, health checks, resource
defaults, port/security documentation, and a CI validation case.

## Common workflows

- Use `list acc` to inspect saved profiles and `doctor` to inspect local CLI
  state. `doctor` does not make a cloud request; rerun `setup oracle` to test
  OCI access.
- Use `instance stop`, `instance restart`, and `instance delete` with either an
  instance name or OCI instance ID. Deletion requires `--yes` in scripts.
- Use `--output json` whenever another program will consume the result.
- Use `plan` to review creates, then pass the returned hash to `apply`. Apply
  recomputes the plan and refuses a changed manifest.
- Use `run --app NAME` or `run --script NAME` to select a validated recipe. The
  recipe is rendered to cloud-init and sent in the OCI launch request; the CLI
  returns when that request completes, while installation and health checks run
  during first boot.

See the [command reference](docs/command-reference.md) for every supported
flag, the [built-in catalog](docs/builtin-catalog.md) for the exact shipped
inventory, the [recipe format](docs/recipe-format.md) for custom applications,
and the [release guide](docs/releasing.md) for maintainers.

## Shell completion

Generate a native completion adapter once and source it from the shell startup
file. The same protocol works for Bash, Zsh, Fish, and PowerShell, including
prefix completion such as `infinity-iac cr<Tab>`:

```sh
# Bash or Zsh (add the corresponding line to ~/.bashrc or ~/.zshrc)
source <(infinity-iac completion bash)
source <(infinity-iac completion zsh)

# Fish (save this as ~/.config/fish/completions/infinity-iac.fish)
infinity-iac completion fish > ~/.config/fish/completions/infinity-iac.fish
```

```powershell
# PowerShell profile
infinity-iac completion powershell | Out-String | Invoke-Expression
```

## Development

```sh
make test
make test-race
make vet
make fmt-check
make validate-recipes
make build
```

GitHub Actions runs formatting, vet, race-enabled tests, recipe validation, and
cross-platform builds for pushes and pull requests. A tag matching `v*.*.*`
builds Linux, macOS, and Windows binaries, creates the Windows setup program,
adds `install.sh`, writes `SHA256SUMS`, verifies all assets, and publishes a
GitHub Release. Go library/module publishing is tracked separately in
`TODO.md`; the current public product is the CLI and its embedded recipe
catalog.

## Scope and limitations

The production provider currently owns OCI compute instances and VNIC public-IP
lookup. Reserved `network`, `sandbox`, `wrap`, and `import` namespaces fail
clearly until their provider layers exist. Recipe ingress entries are validated
metadata; this release does not create OCI security rules. Networking scripts
are rejected rather than run with local or fake behavior.
