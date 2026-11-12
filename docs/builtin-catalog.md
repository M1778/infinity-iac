# Built-in catalog

The current source embeds **137 definitions**: 131 application entries and six
scripts. **80 application demos and six scripts are experimental; 51 application
templates remain blocked.** A concrete definition, supported image manifest,
shell validation, and actual runtime verification are distinct.

The source targets 1.1.0. Installers use published tags, not this checkout. Use the
matching binary/command reference when inspecting a recipe.

## Inspect offline

```sh
infinity-iac list recipes --output json
infinity-iac list capabilities --output json
infinity-iac recipe render --name nginx --port 8080
infinity-iac recipe render --name oc-hunter
infinity-iac run --recipe nginx --dry-run --output json
```

Every definition is discoverable with status and requirements. Unknown names and
templates fail before provisioning. `--inputs FILE` supplies validated typed
values, not executable shell; rendering sensitive values requires explicit
opt-in. Embedded catalog precedence never silently trusts a current-directory
recipes folder.

## Concrete application definitions

The original 19 improved demos are alertmanager, caddy, changedetection,
excalidraw, freshrss, gitea, grafana, jellyfin, mariadb, mysql, n8n, nginx, node-red,
postgres, prometheus, qdrant, redis, registry, and uptime-kuma.

The additional 61 definitions are appsmith, audiobookshelf, baikal, bazarr,
budibase, cassandra, clickhouse, cockroachdb, code-server, coder, consul, couchdb,
dashy, directus, dokuwiki, dolibarr, drupal, duplicati, emby, etcd, filebrowser,
forgejo, ghost, homeassistant, influxdb, jenkins, joomla, jupyter, kafka, kanboard,
keycloak, komga, kopia, loki, mattermost, meilisearch, metabase, mongodb, nats,
netdata, nextcloud, nocodb, ollama, open-webui, owncloud, pgadmin, plex, prefect,
rabbitmq, redmine, searxng, speedtest, stirling-pdf, syncthing, traccar, traefik,
umami, vaultwarden, vikunja, wallabag, and wordpress.

They bind services to loopback, guard existing container/network/volume
ownership, preserve named volumes, and use real protocol or HTTP readiness.
Database-backed recipes supply isolated persistent stacks and wait for DB
readiness before app startup. Bootstrap credentials are generated and persisted
in owner-only guest files; they are not retrieved or printed by the CLI. Some
images are digest-pinned; other explicit version tags still require runtime and
registry verification. Consult each manifest's declared architectures.

Many demos intentionally stop at installation/onboarding. Complete admin
creation, configured domains, authenticated TLS, media/model data, backups, and
production policy yourself before exposing them. Static Excalidraw is not a
collaboration backend; a local registry is not an authenticated public registry;
Caddy's default HTTP demo is not automatic production HTTPS. Development-only
modes and optional app integrations are not production certification.

## Scripts

| Name | Target | Purpose |
| --- | --- | --- |
| system-report | New instance | OS, memory, filesystems, sockets. |
| network-report | New instance | Interfaces, routes, resolver, sockets. |
| disk-report | New instance | Block devices, filesystem usage, logs. |
| docker-report | New instance | Docker diagnostic inventory; no pruning. |
| cloud-init-report | New instance | Status and logs; no reinitialization. |
| oc-hunter | Local | Bounded explicit OCI capacity allocation with persisted identity. |

Guest diagnostic recipes provision a VM; they do not execute on arbitrary
existing servers. Local `oc-hunter` requires `--local` in the recipe wrapper and
uses local credentials, never copying them into cloud-init. The native command
also offers `--dry-run` and explicit launch confirmation.

## Remaining 51 templates

All remaining templates contain application-specific missing prerequisites and
no executable Docker placeholders, bogus health claims, or fictitious outputs.
They are not delivered working integrations.

| Entries | Missing supported deployment work |
| --- | --- |
| argocd, awx, opencost, rancher | Kubernetes control plane, scoped credentials, supported charts/operators, persistence and readiness. |
| authentik, calcom, casdoor, outline | Complete DB/cache/worker dependencies, stable keys and configured identity/calendar/SSO/URLs. |
| dendrite, matrix-synapse, mastodon, peertube, jitsi | Federation/media components, immutable domains, signing secrets, TLS/SMTP and reviewed networking. |
| airflow, chatwoot, discourse, docspell, erpnext, espocrm, immich, paperless-ngx, penpot, rocket-chat, seafile, taiga, temporal | Version-matched service/worker stacks, DB/broker initialization, durable assets and actual background readiness. |
| bookstack, firefly-iii, guacamole, rundeck, superset, xwiki, zabbix | Databases, initialization/encryption/bootstrap and application/worker readiness; guacd/Zabbix components where needed. |
| drone, gitlab, woodpecker | Persistent authenticated CI and forge/agent configuration with authorized execution backends. |
| graylog, kibana, opensearch, sonarqube, weaviate | Matching supported dependencies, authenticated durable storage, kernel/resource limits and readiness. |
| adguard-home, pihole | Authenticated persistent DNS setup, private client/upstream configuration and resolver/capability review. |
| minio, moodle, strapi, vllm | Supported maintained/licensed or custom-built images; vLLM additionally needs authorized models and GPU/runtime testing. |
| vault | Durable non-development TLS/Raft operation and explicit initialization/unseal/root-token custody. |
| phpmyadmin | Authorized DB target, private authentication/session setup and readiness. |
| restic-rest-server | Authenticated durable repository, policy/TLS and restore testing. |
| kuma | Duplicate of canonical uptime-kuma; blocked instead of launching a second pretend integration. |

## Network, bootstrap, and cleanup

```sh
infinity-iac run --recipe nginx --port 8080 --name web-demo --access default --yes
ssh -N -L 8080:127.0.0.1:8080 ubuntu@INSTANCE_PUBLIC_IP
infinity-iac instance delete INSTANCE_ID --access default --yes --wait
infinity-iac network delete NETWORK_ID --access default --yes --wait
```

Managed network commands provide owned VCN/subnet/gateway/routing/security CRUD.
Recipe ingress is not an automatic public-firewall request. Restrict SSH to your
own source and retain outbound package/image access. Use the same access name
for the network and associated compute profile.

The renderer uses bounded commands/health probes, `set -euo pipefail`, private
file defaults, a bootstrap lock, and a readiness marker after successful probes.
Named volumes survive owned container recreation but disappear with VM/boot
volume deletion. Keep backups and exact IDs until cleanup is independently
verified.

## Actual evidence

- Representative amd64 Nginx HTTP/restart and Redis restart/recreation
  persistence passed and test resources were removed.
- November 10, 2026: first-party owned OCI network setup/create/get/SSH update
  passed. Independent dependency-ordered cleanup and absent-resource checks
  passed; personal resources were preserved.
- ARM VM allocation returned `LimitExceeded` for A1 core/memory service limits.
  No test VM or boot volume was created; guest application and compute lifecycle
  tests were not completed.
- Go schema/render/API/CLI/ownership/race tests and 38 documentation browser
  cases passed; two desktop mobile-menu cases were intentionally skipped.

No catalog entry is promoted to fully verified by these representative results.
The remaining runtime/platform and integration gaps stay visible in
[TODO](../TODO.md).
