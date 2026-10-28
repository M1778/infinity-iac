# Built-in catalog

This page is the release inventory for Infinity-IaC 1.0.1. It describes files
embedded into the release binary, not recipes that happen to exist in a local
checkout.

## Inventory

| Recipe | Kind | File | Status |
| --- | --- | --- | --- |
| `n8n` | application | `recipes/n8n/recipe.yaml` | shipped and embedded |

There is no second built-in application and no built-in script recipe in 1.0.1.
The planned catalog of 100+ popular applications and scripts has not been
implemented or shipped. Names such as Docker, PostgreSQL, Redis, WordPress,
Kubernetes, Prometheus, backup, networking, and security scripts are not
valid built-in recipe names in this release.

Confirm the catalog from the installed binary instead of assuming a roadmap
item is available:

```sh
infinity-iac list recipes
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
```

The JSON response includes the catalog source. A release binary should report
`"source": "builtin"` and one recipe named `n8n`. A checkout can report
`"source": "recipes"`; that is the filesystem catalog and is still only the
recipe files present in that checkout.

## n8n application recipe

Run it with:

```sh
infinity-iac run --app n8n --yes
```

Override the host port, instance name, or add an extra first-boot script:

```sh
infinity-iac run --app n8n \
  --name workflow-automation \
  --port 1080 \
  --startup-script ./n8n-extra.sh \
  --output json \
  --yes
```

### What it uses

- **OCI base image:** the image OCID saved by `setup oracle`; the recipe does
  not select or download an Ubuntu image by name.
- **OCI shape:** the shape saved by `setup oracle`; the recipe requests 2 vCPU
  and 2 GB RAM for Flex shapes.
- **Storage:** 35 GB requested by the recipe, raised to OCI's 50 GB boot-volume
  minimum by the provider.
- **Networking:** the configured subnet and a public VNIC. The recipe declares
  TCP port 5678 as public metadata, but 1.0.1 does not create OCI security-list
  or network-security-group rules.

### What runs on first boot

The CLI converts the validated recipe to OCI cloud-init user data. The generated
script performs these operations in order:

1. Updates the Ubuntu package index.
2. Installs the `docker.io` package.
3. Starts `docker.n8n.io/n8nio/n8n` as container `n8n` with Docker restart
   policy `unless-stopped`.
4. Maps the selected host port to container port 5678.
5. Runs the declared HTTP health check against `http://127.0.0.1:<port>/` with
   retries.
6. Appends the optional `--startup-script` content, if supplied.

The OCI launch request returning successfully means the instance was accepted
by OCI. It does not mean Docker, n8n, or the health check has finished. Inspect
the instance's cloud-init output and Docker logs when the service is not ready.

### Declared outputs

The recipe declares a public URL and an `admin_password` secret output. These
are recipe metadata in 1.0.1. Infinity-IaC does not yet discover the public IP
after boot, generate or retrieve the password, or print a ready-to-use URL.
Use `list instances --output json` to obtain the public IP after OCI reports it,
then apply your own n8n authentication and secret handling.

## Why a name may not work

`run --app NAME` and `run --script NAME` both look up `NAME` in the same
validated catalog. Unknown names fail before provisioning. The `--script` flag
does not currently require `kind: script`; it is simply another recipe-name
selector. A custom recipe can be tested without modifying the binary:

```sh
infinity-iac validate recipe --file ./my-recipes/postgres/recipe.yaml
INFINITY_IAC_RECIPES=./my-recipes infinity-iac list recipes
INFINITY_IAC_RECIPES=./my-recipes infinity-iac run --app postgres --yes
```

Custom recipes are not built-ins. To ship one in a release, add its validated
recipe file under `recipes/<name>/`, include it in `recipes/embed.go`, add tests,
and update this inventory.

## Planned catalog

The 100+ catalog remains a TODO. Each recipe needs supported OCI image and
architecture metadata, resource sizing, package or container provenance,
idempotent bootstrap commands, health checks, ingress requirements, outputs,
cleanup guidance, and CI coverage before it can be called production-ready.
