# Infinity-IaC command reference

This reference describes the 1.1.0 source. Check the installed binary with
`infinity-iac --version`; installers use published tags, not the checkout.

## Configuration

`INFINITY_IAC_CONFIG` selects an isolated local profile file. The default is the
platform user config directory. Private OCI keys remain in the standard OCI
credential file, never in Infinity-IaC state, recipes, or cloud-init.

```sh
infinity-iac setup oracle --autodetect --name default --profile DEFAULT \
  --compartment COMPARTMENT_OCID --subnet SUBNET_OCID \
  --availability-domain AVAILABILITY_DOMAIN --image IMAGE_OCID \
  --shape VM.Standard.A1.Flex --ssh-public-key "$(cat ~/.ssh/id_ed25519.pub)"
```

Setup flags: `--name`, `--profile`, `--account`, `--config`, `--region`,
`--compartment`, `--subnet`, `--availability-domain`, `--image`, `--shape`,
`--ssh-public-key`, `--autodetect`, `--network-only`.

`--config` takes precedence over `OCI_CONFIG_FILE` and default discovery.
Autodetection checks readable current-user/sudo-caller/root OCI configs and derives
region and tenancy from the exact selected profile. Invalid explicit paths or
missing profiles fail without silent fallback. Setup performs read-only checks.

A network-only profile requires a named OCI profile, credentials, region, and
compartment, not an image, shape, subnet, or availability domain:

```sh
infinity-iac setup oracle --autodetect --network-only \
  --name default --compartment COMPARTMENT_OCID
```

Use the **same access name** when adding full compute configuration for a managed
subnet. Cross-profile managed subnet adoption is rejected.

`list acc`/`list access` show local profiles; `doctor` reports local configuration.
`unlink acc NAME --yes` removes only local association, not cloud resources or
OCI credentials. `--output json` is available where supported.

## Networks

```sh
infinity-iac network create --name demo-net --access default \
  --cidr 10.88.0.0/16 --subnet-cidr 10.88.1.0/24 \
  --ssh-source YOUR_PUBLIC_IP/32 --yes --output json
infinity-iac network list --access default --output json
infinity-iac network get NETWORK_ID --access default
infinity-iac network update NETWORK_ID --access default \
  --ssh-source YOUR_PUBLIC_IP/32 --yes
infinity-iac network delete NETWORK_ID --access default --yes --wait
```

`create network` and `list networks` are aliases. Actions accept `--access`,
`--output table|json`, `--wait`, and `--timeout` (default 10m, maximum 1h).
Creation additionally takes `--name`, `--cidr`, `--subnet-cidr`, and `--ssh-source`.
Update changes SSH ingress only. Deletion confirms completion regardless of the
optional `--wait` flag.

Networks own VCN, regional subnet, internet gateway, and default route/security/
DHCP resources. CIDRs must be canonical private IPv4; SSH source must be an
explicit /24–/32, normally your address /32. App ports are not opened. Mutations
verify access/compartment/resource association and current ETags. Delete refuses
foreign or occupied dependencies; delete instances/VNICs first. Partial creation
reports known IDs and attempts bounded cleanup.

## Compute

```sh
infinity-iac create server --name demo --access default \
  --cpu 2 --ram 4GB --storage-size 50GB --yes
infinity-iac list instances --access default --output json
infinity-iac instance get INSTANCE_ID --access default
infinity-iac instance stop INSTANCE_ID --access default --yes --wait
infinity-iac instance start INSTANCE_ID --access default --yes --wait
infinity-iac instance restart INSTANCE_ID --access default --yes --wait
infinity-iac instance delete INSTANCE_ID --access default --yes --wait
```

Create flags: `--name`, `--os`, `--cpu`, `--ram`, `--storage-size`, `--provider`,
`--access`, `--region`, `--startup-script`, `--port`, `--amount`, `--yes`, `--output`.
`--amount` is 1–100. `--networking-script` is rejected rather than executing fake
networking. The profile's image determines the guest OS. CPU requests are vCPUs:
A1 uses one per OCPU and x86 Flex two; provider minimums may increase allocation.
Boot storage is at least 50 GB.

Instance actions accept names or exact IDs, `--access`, `--yes`, `--wait`,
`--timeout` (10m default, at most 1h), and `--output`. Flags may precede or follow
the identifier. Ambiguous names fail. All noninteractive mutations require
`--yes`; direct provider mutations additionally reject unowned/cross-profile
resources. Legacy unscoped managed records are not silently adopted.

Restart starts stopped instances and soft-resets running ones. Delete without
`--wait` reports `terminating`/`confirmed:false`; with `--wait` it confirms
termination and recorded boot-volume deletion. Local Docker volumes disappear
with that boot volume. Back up state first.

## Recipes and execution location

The embedded source catalog contains 137 definitions: 131 apps and six scripts.
80 apps and six scripts are experimental; 51 app templates remain blocked.
Concrete definition, shell syntax, image metadata, and actual runtime verification
are different things. See [catalog scope](builtin-catalog.md).

```sh
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
infinity-iac validate recipe --file ./recipe.yaml
infinity-iac recipe render --name nginx --port 8080
infinity-iac recipe render --file ./recipe.yaml --inputs values.json
infinity-iac run --recipe nginx --access default --name demo --yes
infinity-iac run --app nginx --inputs values.yaml --dry-run --output json
infinity-iac run --script oc-hunter --local --inputs hunter.json --dry-run
```

`recipe list` aliases `list recipes`; `recipe validate` aliases `validate recipe`.
Catalog selection: explicit `--root` for inspection, `INFINITY_IAC_RECIPES`, then
embedded definitions. Installed apps are appended only to the normal catalog.
Current-directory recipes never silently replace embedded commands.

Render requires exactly one `--name`/`--file`, and accepts `--port`, `--inputs`,
`--include-sensitive`. It performs no cloud call or command execution. Supplied
sensitive inputs require explicit render opt-in and a private output destination.

Run requires exactly one `--app`, `--script`, or `--recipe`. App/script selectors
enforce kind. Options include `--name`, `--access`, `--inputs`, `--port`,
`--specs auto`, `--startup-script`, `--local`, `--dry-run`, `--yes`, and `--output`.
Guest recipes create a new instance. Local scripts require `--local`; guest
bootstrap may not be run locally through that flag. Dry-run validates the
execution definition without allocating or executing, but does not certify a
live image, account permission, or application behavior.

Typed input objects validate required/default values, string/integer/boolean
types, ranges, and unknown keys. Strings are exported as safely quoted
`INFINITY_INPUT_*` variables; only numeric `{{port}}` is raw substitution. See
[recipe format](recipe-format.md). Guest commands run with bounded steps/health
checks and write `/var/lib/infinity-iac/ready/NAME` only after completion.

## Local oc-hunter

An independent local implementation of the capacity-allocation concept in
[the reference repository](https://github.com/mohankumarpaluru/oracle-freetier-instance-creation),
not its unsafe first-subnet selection or shape-only instance adoption.

```sh
infinity-iac oc-hunter --access default --name capacity-demo --dry-run
infinity-iac oc-hunter --access default --name capacity-demo \
  --availability-domains EXACT_AD_1,EXACT_AD_2 \
  --cpu 2 --ram 12GB --storage-size 50GB \
  --interval 60s --attempts 20 --timeout 30m --yes
```

Native flags: `--access`, `--name`, `--availability-domains`, `--shape`, `--cpu`,
`--ram`, `--storage-size`, `--interval`, `--attempts`, `--timeout`, `--state-file`,
`--dry-run`, `--yes`, `--output`. Allowed shapes: A1 Flex and E2 Micro. Fixed
Micro allocation requires `--cpu 1 --ram 1GB`. Domain names must be exact;
configuration is taken from the chosen profile, never an arbitrary subnet.

Interval is 30s–30m; attempts 1–100; timeout positive and at most 8h. Capacity
errors rotate domains; throttling backs off. Quota/auth/config faults stop.
Unknown launch outcomes remain pending and stop; failed reconciliation is not
permission to launch again. Only exact managed access/name/operation-token
inventory is reconciled. Persisted 0600 state and a lock prevent duplicate and
concurrent operations. Retain pending state and investigate before retrying.
There is no scheduler, credential copying, notification transport, or cost
eligibility guarantee.

## Plans, output, and local tools

```sh
infinity-iac plan --file infrastructure.yaml --output json
infinity-iac apply --file infrastructure.yaml --hash REVIEWED_HASH --yes
infinity-iac completion bash
infinity-iac completion zsh
infinity-iac completion fish
infinity-iac completion powershell
infinity-iac update --yes
infinity-iac install skills --dry-run
```

Manifest v1 supports `servers` with name/provider/access/region/OS/CPU/RAM/storage/
port/startup-script/user-data fields. Missing specs normalize to defaults. Plans
are scoped to one selected profile/provider/region; existing names match that
scope, not names from other profiles. Startup file contents affect the hash.
Apply supports creates only, not modification or implicit destruction.

Table is default; supported JSON is indented stdout. Errors use stderr and
nonzero status. `--yes` confirms a request, not authorization or a bypass of
provider protections. Completion supports implemented commands only. Unsupported
sandbox, backup, wrap, and import features are not silently emulated.

## Troubleshooting

- Missing flag: inspect binary version and explicit path, particularly under sudo.
- Unreadable credentials: fix ownership/config/profile/key references; never make
  private keys world-readable or copy them into CI/cloud-init.
- Running VM but failed app: inspect cloud-init, the ready marker, outbound
  package/image access, and real loopback HTTP/database health; use an SSH tunnel
  rather than opening app ingress.
- `LimitExceeded`: check tenancy quotas and limits. Capacity retry does not solve
  a service limit; do not terminate personal resources to free a test slot.
- Pending hunter: inspect exact launch operation; do not delete state merely to
  retry an ambiguous request with a new identity.
- Asynchronous delete: distinguish acceptance from completion and verify exact
  instance, boot volume, subnet, gateway, and VCN states.
