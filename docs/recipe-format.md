# Recipe format

Recipes are YAML or JSON with `api_version: recipe/v1`, a unique safe name,
version, kind, supported targets, and at least one step. They are validated
before rendering or launching. Templates require application-specific
`requirements` and are always blocked.

## Catalog and execution

The current embedded catalog has 137 definitions: 131 application entries and
six scripts. 80 app demos and six scripts are experimental; 51 templates remain
incomplete. Source counts are not runtime certification. The installer downloads
published tags, not the checkout.

Resolution uses an explicit inspection `--root`, then `INFINITY_IAC_RECIPES`, then
embedded recipes and locally installed apps. A current-directory recipes folder
never silently replaces executable definitions.

`execution` defaults to `instance`. `execution: local` is allowed only for script
recipes and uses explicit local execution; it never provisions a VM. Local
recipes support command steps only, not implicit package/file writes.

## Schema example

```yaml
api_version: recipe/v1
name: demo
version: 1.0.0
kind: application
execution: instance
status: experimental
description: Configured loopback service
supports:
  providers: [oracle]
  os: [ubuntu-24.04, ubuntu-22.04]
  architectures: [amd64, arm64]
inputs:
  port:
    type: integer
    default: 8080
    min: 1
    max: 65535
  message:
    type: string
    required: true
  token:
    type: string
    required: true
    sensitive: true
resources:
  cpu: 2
  ram_gb: 4
  storage_gb: 50
  ingress:
    - port: 8080
      protocol: tcp
      source: loopback
steps:
  - id: package
    type: package
    package: curl
  - id: config
    type: file
    destination: /etc/demo.conf
    mode: "0600"
    content: |
      listen=127.0.0.1
  - id: start
    type: command
    command: /usr/local/bin/demo --port {{port}} --message "$INFINITY_INPUT_MESSAGE"
    timeout_seconds: 180
health:
  - type: http
    port: 8080
    path: /health
    timeout_seconds: 120
outputs:
  url:
    type: string
    value: http://127.0.0.1:{{port}}
```

This is a format example, not a shipped demo executable.

## Typed inputs and secrets

Input names use lowercase letters, digits, and underscores. Types are `string`,
`integer`, and `boolean`. Values come from a JSON/YAML object through `--inputs`.
Unknown fields, wrong types, out-of-range integers, missing/empty required
strings, NUL bytes, and oversized text fail before execution. Sensitive inputs
cannot declare defaults in source.

Values are exported as shell-quoted `INFINITY_INPUT_<UPPERCASE_NAME>` variables.
An explicit `environment` must remain in that reserved namespace and be unique.
Commands must quote variable references. User text is never expanded as shell
syntax; only numeric `{{port}}` substitution is supported. Other raw text tokens
are rejected. Literal file `content` is base64 encoded, not evaluated or
interpolated.

```sh
infinity-iac recipe render --file recipe.yaml --inputs values.json
infinity-iac run --recipe demo --inputs values.yaml --dry-run
```

Rendering supplied sensitive values requires `--include-sensitive`; protect the
resulting output file. Normal errors must not echo input values. Rendered scripts
and provider metadata can contain supplied credentials: never publish them or
send them to shared logs. Catalog outputs describe metadata, not retrieved guest
secrets.

## Steps, limits, and health

- `package`: validates a package identifier, batches apt update/install with
  bounded waits and lock timeouts, and enables Docker for `docker.io`.
- `command`: executes reviewed commands with `set -euo pipefail`; declared
  `timeout_seconds` wraps a bounded shell.
- `upload`: reads `source` on the caller, writes base64-decoded bytes to an
  absolute guest destination, and applies the selected mode.
- `file`: writes literal inline content to an absolute guest destination.

File mode defaults to `0600`; allowed modes are `0600`, `0640`, `0644`, `0700`,
and `0755`. Symlink destinations are rejected. Local scripts cannot declare
file/upload/package steps. Remote third-party upload bundles are rejected;
local app installation copies absolute upload sources into private storage.

Guest bootstraps serialize with `flock`, clear stale readiness markers, and run
bounded HTTP or command probes. HTTP requests use loopback and connect/request
timeouts. Command attempts are bounded. Completion writes
`/var/lib/infinity-iac/ready/NAME`; failure does not. OCI launch acceptance is not
application readiness.

Resources request vCPU/RAM/storage from the configured profile. A1 and x86 OCPU
ratios differ; OCI minimums may raise actual allocation, including 50 GB boot
storage. `supports` declares intended targets and is preflight-checked, not a
claim of runtime certification. `ingress` remains recipe metadata: managed
network commands separately provision reviewed SSH/outbound rules, not public
application ports.

## Run and persistence

```sh
infinity-iac run --app nginx --access default --port 8080 --yes
infinity-iac run --script system-report --name diagnostics --yes
infinity-iac run --script oc-hunter --local --inputs hunter.json --dry-run
```

Exactly one selector is required; app/script selectors enforce kind. Instance
recipes create a new VM. Local recipes require `--local` for real execution.
`--dry-run` validates without allocation or command execution; it does not prove
cloud permissions, image availability, or runtime functionality.

Use ownership checks before recreating containers, bridge networks, or named
volumes. Dependent apps must wait for authenticated DB/broker readiness before
launching, not merely at the final health stage. Named volumes survive container
recreation but not deleting the VM/boot volume. Complete administrator setup,
TLS, backups, and appropriate network policy before production use.

`experimental` is controlled evaluation. `verified` requires exact version,
platform, readiness, functional behavior, and persistence/recovery evidence.
Untested domains, GPUs, clusters, or custom builds must remain explicit
prerequisites or blocked templates, not pass by omission.
