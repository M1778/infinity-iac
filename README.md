# Infinity-IaC

Infinity-IaC is a Go CLI and importable Go package for Oracle Cloud Infrastructure
(OCI). It manages explicitly owned instances and networks, renders application
recipes to cloud-init, and provides hash-bound plans and a local capacity helper.

## Version and scope

The source targets **1.1.1**. Installers download published GitHub Release assets,
not the current checkout. Check the release page before selecting a tag; use an
explicit source-built binary for changes not yet released.

The embedded catalog contains **137 definitions**: 131 application entries and
six scripts. **80 application demos and six scripts are experimental; 51
application templates remain blocked.** Experimental means a concrete evaluation
definition, not production or cross-architecture certification. Dependencies and
remaining prerequisites are documented per entry.

[Documentation website](https://m1778.github.io/infinity-iac/) ·
[Command reference](docs/command-reference.md) ·
[Catalog and runtime scope](docs/builtin-catalog.md)

## Install

### Linux and macOS

The installer supports amd64 and arm64, verifies `SHA256SUMS`, and defaults to a
user-writable prefix. Download and inspect the installer before executing it:

```sh
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh -o install.sh
sh install.sh --prefix "$HOME/.local/bin"
"$HOME/.local/bin/infinity-iac" --version
```

Use `--version v1.1.1` only after that release is published. The corresponding
environment settings are `INFINITY_IAC_VERSION`, `INFINITY_IAC_PREFIX`, and
`INFINITY_IAC_REPOSITORY`.

### Windows

Download `infinity-iac-setup.exe` from
[GitHub Releases](https://github.com/M1778/infinity-iac/releases). The per-user
installer offers a user `PATH` entry. Open a new terminal after installation.
Uninstalling the CLI does not delete cloud resources.

### Build from source

Go 1.23 or newer is required:

```sh
go build -trimpath -o bin/infinity-iac ./cmd/infinity-iac
./bin/infinity-iac --version
```

When using root-owned OCI credentials, select the binary explicitly, for example
`sudo ./bin/infinity-iac …`; root may otherwise resolve an older executable.

## Configure OCI

Configure the standard OCI CLI credential file first. Infinity-IaC stores the
profile reference and resource identifiers, never the private key. Setup validates
access with read-only API calls and creates no cloud resources.

```sh
export INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json"
infinity-iac setup oracle --autodetect --name default --profile DEFAULT \
  --compartment COMPARTMENT_OCID --subnet SUBNET_OCID \
  --availability-domain AVAILABILITY_DOMAIN --image IMAGE_OCID \
  --shape VM.Standard.A1.Flex \
  --ssh-public-key "$(cat ~/.ssh/id_ed25519.pub)"
infinity-iac list acc
infinity-iac doctor --output json
```

`--config` takes precedence over `OCI_CONFIG_FILE` and automatic discovery.
Invalid explicit paths and missing named OCI profiles fail without fallback.
Autodetection derives region and tenancy; it does not choose a shape or guess
subnet, image, or availability-domain identifiers.

## Managed networks

A network-only profile needs credentials, region, and compartment, not an
existing subnet or compute image:

```sh
infinity-iac setup oracle --autodetect --network-only \
  --name default --compartment COMPARTMENT_OCID
infinity-iac network create --name demo-net --access default \
  --cidr 10.88.0.0/16 --subnet-cidr 10.88.1.0/24 \
  --ssh-source YOUR_PUBLIC_IP/32 --yes --output json
infinity-iac network get NETWORK_ID --access default
infinity-iac network update NETWORK_ID --access default \
  --ssh-source YOUR_PUBLIC_IP/32 --yes
```

Use the returned subnet in a full compute setup **with the same access name**.
Managed resources are bound to that profile. A network owns its VCN, regional
subnet, internet gateway, and default routing/security/DHCP resources. SSH must
use an explicit narrow source; application ports are not opened automatically.
Updates use ETags, and deletion refuses foreign or occupied dependencies.

```sh
infinity-iac network list --access default --output json
infinity-iac network delete NETWORK_ID --access default --yes --wait
```

Network creation and deletion confirm completion. `--timeout` bounds operations;
network deletion must follow instance/VNIC cleanup.

## Instances and recipes

```sh
infinity-iac create server --name demo --access default \
  --cpu 2 --ram 4GB --storage-size 50GB --yes
infinity-iac list instances --access default --output json
infinity-iac run --recipe nginx --name web-demo --access default \
  --port 8080 --yes
```

`--cpu` requests vCPUs: A1 uses one vCPU per OCPU; x86 Flex uses two. The
provider may raise requests to its minimum allocation. Boot storage is at least
50 GB. The configured image OCID, not an arbitrary OS label, selects the guest.
Recipe support is checked before launch.

Inspection works offline:

```sh
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
infinity-iac recipe render --name nginx --port 8080
infinity-iac run --recipe nginx --dry-run --output json
```

Use exactly one of `--app`, `--script`, or `--recipe`. App/script selectors enforce
kind. Instance-target recipes create a VM; local scripts require explicit
`--local`. Typed JSON/YAML inputs use `--inputs FILE`; required fields, types,
and ranges are validated. Rendering supplied sensitive values additionally
requires `--include-sensitive` and a protected output file.

```sh
infinity-iac instance get INSTANCE_ID --access default
infinity-iac instance stop INSTANCE_ID --access default --yes --wait
infinity-iac instance start INSTANCE_ID --access default --yes --wait
infinity-iac instance restart INSTANCE_ID --access default --yes --wait
infinity-iac instance delete INSTANCE_ID --access default --yes --wait
```

Mutations reject unowned and cross-profile resources. Restart starts stopped
instances and soft-resets running instances. Without `--wait`, deletion reports
acceptance as `terminating`; confirmed deletion waits for termination and recorded
boot-volume deletion. This removes local Docker volumes, so back up data first.

Services bind to loopback by default. Connect through an SSH tunnel:

```sh
ssh -N -L 8080:127.0.0.1:8080 ubuntu@INSTANCE_PUBLIC_IP
```

An onboarding page is not completed authentication or TLS configuration.

## Local oc-hunter

`oc-hunter` independently implements the capacity-hunting concept from
[oracle-freetier-instance-creation](https://github.com/mohankumarpaluru/oracle-freetier-instance-creation).
It uses existing local OCI credentials, explicit images/subnets, and the A1 Flex
or E2 Micro shape. It never copies credentials to a guest, installs a scheduler,
or guarantees free-tier eligibility.

```sh
infinity-iac oc-hunter --help
infinity-iac oc-hunter --access default --name capacity-demo --dry-run
infinity-iac oc-hunter --access default --name capacity-demo \
  --interval 60s --attempts 20 --timeout 30m --yes
infinity-iac recipe render --name oc-hunter
infinity-iac run --script oc-hunter --local --inputs hunter.json --dry-run
```

Capacity failures rotate selected domains conservatively. Throttling backs off;
authentication, quota, and unknown failures stop. A private persisted journal
records per-domain retry tokens before launch, prevents concurrent use, and
reconciles only the exact owned operation. Retain that journal after interruption;
uncertain responses must not be followed by a new launch identity.

## Plans and Go API

```sh
infinity-iac plan --file infrastructure.yaml --output json
infinity-iac apply --file infrastructure.yaml --hash REVIEWED_HASH --yes
```

Plans cover implemented create actions, not implicit deletion of omitted
resources. Startup file content affects the reviewed hash. Plans are scoped to
the selected profile, provider, and region.

```go
import infinityiac "github.com/M1778/infinity-iac"

catalog, err := infinityiac.BuiltinCatalog()
item, ok := catalog.Get("nginx")
script, err := infinityiac.RenderCloudInit(item, infinityiac.RenderOptions{Port: 8080})
```

The snippet shows entry points, not complete error handling. The root package
also exposes `NewPlan`, typed inputs, and guarded compute/network constructors.
Use deadlines and confirmation at your application's boundary.

## Development and verification

```sh
make fmt-check vet test-race validate-recipes build
go run ./cmd/docs-data --check
npm ci --ignore-scripts
npx --no-install playwright install --with-deps chromium
npm run test:docs
sh -n install.sh scripts/verify-release.sh
git diff --check
```

CI checks Linux, macOS, Windows, and the documentation experience without OCI
credentials. Pages publishes only after validation. Release tags build five
platform binaries and installers and verify checksums and version alignment.

**Actual live scope:** owned OCI network creation, inspection, SSH-rule updates,
and deletion were exercised and cleanup verified on November 10, 2026. Compute
launch was refused with `LimitExceeded` for A1 core/memory limits; no test VM or
boot volume was created. Personal resources were preserved. Representative amd64
Nginx HTTP/restart and Redis restart/recreation persistence tests passed. These
results do not certify all catalog entries or ARM deployment.

See [remaining prerequisites](TODO.md), [recipe format](docs/recipe-format.md),
and [release instructions](docs/releasing.md). Unsupported sandbox, wrapper,
backup, import, and arbitrary networking-script features are not silently
emulated.
