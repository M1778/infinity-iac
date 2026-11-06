# Built-in catalog

The current source embeds **136 recipe definitions: 131 application entries and five diagnostic scripts**. This is not a claim that 131 applications have been deployed successfully. There are 19 standalone application demos and five scripts marked `experimental`; 112 incomplete entries are marked `template` and are rejected by both `run` and `recipe render` before provisioning.

The published v1.0.1 release predates this expansion. `install.sh` downloads a published release, not the current source branch. Build from the branch to use these changes until they are included in a tagged release.

## Inspect without provisioning

```sh
infinity-iac list recipes
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
infinity-iac recipe render --name nginx --port 8080 > nginx-cloud-init.sh
infinity-iac recipe render --name system-report > diagnostics-cloud-init.sh
```

The catalog is validated for schema and duplicate names. Validation and shell rendering do not prove an application starts, handles requests, preserves data, or supports every advertised architecture. `experimental` means the deployment definition is available for evaluation, not production certified. `template` means required configuration or services are still missing. See the requirements returned in the JSON inventory before choosing an entry.

The catalog always uses embedded recipes and locally installed recipes unless you explicitly select `--root` for inspection or `INFINITY_IAC_RECIPES` for development. An arbitrary `recipes/` directory in the current working directory is not implicitly executed.

## Standalone application demos

The improved application batch contains nginx, caddy, grafana, prometheus, redis, postgres, mysql, mariadb, qdrant, n8n, node-red, jellyfin, gitea, freshrss, changedetection, alertmanager, uptime-kuma, excalidraw, and registry.

Their service ports bind to `127.0.0.1`. Stateful services use stable named Docker volumes. PostgreSQL, MySQL, MariaDB, and Qdrant generate bootstrap secrets in owner-only files under `/var/lib/infinity-iac/<recipe>/credentials.env`. The files are local to the remote instance and are not retrieved or printed by the CLI. n8n no longer advertises a fictitious generated administrator password; complete its onboarding through the local connection.

Several recipes intentionally start at an installer or onboarding screen. For example, Gitea and FreshRSS need application setup, Jellyfin needs an administrator and media mounts, Uptime Kuma needs its initial administrator and a monitor, and Caddy needs explicit domain/configuration/443 setup to provide automatic HTTPS. Excalidraw is a static frontend demo, not an installed collaboration backend. Registry is a local-only registry without an authenticated public publication layer. Set administrator passwords and configure authenticated TLS before making any of these accessible beyond an SSH tunnel.

## Diagnostic scripts

| Recipe | Purpose |
| --- | --- |
| `system-report` | Operating system, memory, filesystem, and listening socket diagnostics. |
| `network-report` | Interfaces, routes, resolver configuration, and listening ports. |
| `disk-report` | Block devices, filesystem capacity, and log-directory size. |
| `docker-report` | Docker daemon, container status, and disk usage; it never prunes resources. |
| `cloud-init-report` | Current cloud-init status and recent cloud-final logs without reinitializing. |

`run --script NAME` provisions a new instance and executes the script at first boot; it is not a command for running scripts on an existing server. `recipe render` is an offline inspection command. Review a rendered script before applying it yourself. The diagnostics themselves are read-only, but provisioning and any declared package installation are not.

## Incomplete application templates

Large multi-service deployments such as Authentik, Immich, Paperless-ngx, Outline, BookStack, WordPress, Penpot, Jitsi, and Zabbix need databases, brokers, workers, or backend services not supplied by the old single-image commands. Kubernetes applications such as Argo CD and AWX need a supported control-plane deployment. Other entries need explicit startup arguments, configuration, supported image versions, GPU hardware, or appropriate resource sizing.

These entries remain discoverable so the catalog is an honest inventory of planned integrations, but they cannot be launched until completed. Known examples include:

- Budibase's old container mapping should target port 80, not 10000, and its memory requirement is larger than the generic two GB preset.
- Penpot's frontend listens on 8080, not the old 9001 mapping; the frontend alone is not Penpot.
- Coder needs an explicit listener and access URL plus durable database/configuration.
- Discourse and Moodle need a supported replacement for the unavailable Bitnami `latest` references.
- Docspell needs restserver, joex, database, and search services; the old monolithic reference is not a supported deployment.
- Strapi's old image is legacy v3; supported applications need a new application image.
- OpenSearch needs a strong bootstrap password, discovery settings, host kernel limits, suitable RAM, and authenticated TLS probes.
- Vault's image defaults to development mode and must not be treated as a safe public secrets server.
- The `kuma` entry duplicates Uptime Kuma; use the canonical `uptime-kuma` recipe.

## Provision and connect safely

After configuring a real OCI access profile:

```sh
infinity-iac run --recipe nginx --port 8080 --name web-demo --yes
infinity-iac run --app postgres --name database-demo --yes
infinity-iac run --script system-report --name diagnostics-demo --yes
```

Use exactly one of `--app`, `--script`, or `--recipe`. Unknown names and incomplete templates fail before a create request. `--port` changes both the Docker mapping and protocol probe.

Connect to a loopback service through an SSH tunnel using the instance's configured SSH key:

```sh
ssh -N -L 8080:127.0.0.1:8080 ubuntu@INSTANCE_PUBLIC_IP
```

Recipe ingress is metadata, not an OCI firewall API. The current provider does not create VCNs, subnets, routes, gateways, or security-list rules. Restrict SSH in the existing subnet to your own source IP; do not open the application port publicly just to test it. The instance needs outbound networking to download packages and images.

## Bootstrap and cleanup

The renderer uses `set -euo pipefail`, serializes bootstraps with `flock`, applies declared command timeouts, waits for bounded HTTP/command readiness checks, and records successful completion in `/var/lib/infinity-iac/ready/<recipe>`. A stale completion marker is removed before a new attempt. OCI launch acceptance is not application readiness; inspect cloud-init and Docker logs.

Named volumes survive container recreation, but they do not survive terminating the OCI instance and deleting its boot volume. Back up data separately before instance deletion. CI validates the catalog, rejects incomplete templates, renders runnable entries, checks generated shell syntax, and checks private bindings/persistence requirements without cloud credentials. OCI authentication or schema validation must never be presented as a successful live application test.
