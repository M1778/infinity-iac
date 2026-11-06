# Infinity-IaC

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

`install.sh` installs a tagged GitHub Release, not the current checkout. The
published v1.0.1 release predates the catalog and setup fixes in this source.
Build from source to test unreleased changes. When using sudo, use the explicit
binary path: `sudo ./bin/infinity-iac ...` avoids an older system-wide binary.

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

Add `--autodetect` to find a readable OCI config and derive the selected
profile's region and tenancy unless explicitly overridden. An explicit
`--config` or `OCI_CONFIG_FILE` takes precedence and is not silently replaced
if invalid. Default discovery checks the current user's config, a sudo caller's
config when applicable, and `/root/.oci/config` when readable. Private keys are
not copied or printed. Compartment, subnet, availability domain, image, and
shape are still required; autodetection does not create a VCN or guess them.

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

The current source embeds **136 definitions: 131 application entries and five
scripts**. This is not a claim that 131 applications are ready to deploy. The
catalog has 19 standalone application demos and five diagnostic scripts marked
`experimental`. The other 112 entries are incomplete `template` recipes and
fail before provisioning. See [the catalog guide](docs/builtin-catalog.md) for
requirements and deployment limitations.

Runnable applications default to loopback-only Docker ports, use persistent
named volumes where state is needed, and check HTTP/database protocol readiness
rather than just process liveness. PostgreSQL, MySQL, MariaDB, and Qdrant
bootstrap random credentials in owner-only files on the instance. Onboarding,
authentication, backups, and HTTPS still need review before production use.

Inspect and render without creating cloud resources:

```sh
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
infinity-iac recipe render --name nginx --port 8080 > nginx-cloud-init.sh
infinity-iac recipe render --name system-report > system-report.sh
```

After setup, provision a selected recipe:

```sh
infinity-iac run --recipe nginx --port 8080 --name web-demo --yes
infinity-iac run --app postgres --name database-demo --yes
infinity-iac run --script system-report --name diagnostics-demo --yes
```

Use exactly one of `--app`, `--script`, or `--recipe`. `run` always creates a new
OCI instance; `recipe render` is the offline preview. Script recipes currently
provide system, network, disk, Docker, and cloud-init reports. Access a remote
loopback web service through SSH, for example:

```sh
ssh -N -L 8080:127.0.0.1:8080 ubuntu@INSTANCE_PUBLIC_IP
```

Validation and rendering are not runtime certification. Cloud-init success is
recorded in `/var/lib/infinity-iac/ready/<recipe>` after bounded health checks.
Inspect bootstrap logs and application functionality before relying on a
service. Deleting an OCI instance deletes its boot volume and any local Docker
volumes, so back up state separately.

## Common workflows

- Use `list acc` to inspect saved profiles and `doctor` to inspect local CLI
  state. `doctor` does not make a cloud request; rerun `setup oracle` to test
  OCI access.
- Use `unlink acc NAME --yes` to remove a saved local access profile. This
  leaves the OCI CLI credentials and all remote resources unchanged.
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

### Install a third-party app recipe

Third-party recipes are intentionally unsafe: their steps can execute arbitrary
commands as root during first boot. Review the manifest and pass --yes in
automation. Installation stores the validated recipe locally and never runs it.

~~~sh
infinity-iac app install owner/repository --yes
infinity-iac app install --external https://example.com/app.yml --yes
infinity-iac app install ./google-app.yml --yes
infinity-iac list recipes
~~~

GitHub repositories must expose app.yml, app.yaml, recipe.yml, recipe.yaml, or
recipe.json at their root. External URLs must use HTTPS. Remote recipes
containing upload steps are rejected because their referenced files are not
available to the CLI. Local upload sources are copied into the private
installed-app directory.

### Update

Update the running CLI from a published GitHub Release. The binary and
SHA256SUMS are downloaded and verified before an atomic replacement.

~~~sh
infinity-iac update --yes
infinity-iac update --version v1.0.1 --yes
~~~

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
