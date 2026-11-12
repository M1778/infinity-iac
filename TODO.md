# Infinity-IaC development TODO

## Implemented

- [x] Checksum-verified Unix installer and per-user Windows installer.
- [x] Tag-triggered five-platform binary, installer, and checksum publication.
- [x] Source/tag/binary version checks before release publication.
- [x] Standard OCI config autodetection with exact named-profile validation.
- [x] Guarded compute get/create/list/start/stop/restart/delete and bounded waits.
- [x] Confirmed instance deletion checks recorded boot-volume deletion.
- [x] First-party managed VCN/subnet/gateway/default routing/security/DHCP lifecycle.
- [x] Profile ownership, ETag mutations, and foreign/occupied dependency rejection.
- [x] Importable root Go API and external-package tests.
- [x] Typed recipe inputs, sensitive-render opt-in, literal file steps, and local scripts.
- [x] Local bounded `oc-hunter`, persisted launch identity, and offline safety tests.
- [x] Embed 137 definitions: 131 application entries and six scripts.
- [x] Implement 80 experimental application demos; 61 replace former templates.
- [x] Replace generic prerequisites and dead launches in all remaining blocked templates.
- [x] Private bindings, owned persistence, authenticated database dependency waits,
  protocol readiness, schema validation, and generated-shell tests.
- [x] Responsive Fluent/Rust-style documentation with catalog search, dark mode,
  keyboard navigation, copy controls, and browser tests.
- [x] Validate generated public catalog and browser behavior before Pages deployment.
- [x] Exercise first-party live OCI network creation/read/update and verify deletion
  through independent cleanup; preserve personal resources.

## Actual verification scope

- Full local Go race tests, static analysis, renderer/API/CLI tests passed.
- Documentation browser suite: 38 passed; two desktop-only skips for mobile-menu
  cases. This covers desktop/mobile light/dark, filters, links, focus, storage
  failures, clipboard failures, and no-JavaScript content.
- Representative amd64 Nginx HTTP/restart and Redis restart/container-recreation
  persistence checks passed; their test resources were removed.
- November 10, 2026: managed OCI VCN/subnet/gateway creation, inspection, and SSH
  update passed. Cleanup was independently verified and personal resources
  remained intact.
- ARM VM launch was refused with OCI `LimitExceeded` for A1 core and memory
  limits. No test instance or boot volume was created. Live cloud-init,
  stop/start/restart/delete, and guest app tests therefore remain unverified.
- The OCI SDK in-flight cancellation race is handled at the request/transport
  boundary and covered by an actual in-flight race-enabled regression test.

## Not completed or not certified

- [ ] Complete the **51** remaining templates with supported, version-matched
  stacks, bootstrap procedures, and real protocol/worker readiness. Catalog
  entries contain application-specific prerequisites and remain fail-closed.
- [ ] Runtime-test every concrete application on the declared platforms before
  labeling it verified. Image-manifest availability and shell tests alone are
  insufficient.
- [ ] Exercise live VM lifecycle and ARM guest applications after the account has
  sufficient authorized service limits. Do not terminate a personal VM to free
  capacity, bypass quotas, or retry `LimitExceeded` indefinitely.
- [ ] Validate GPU, cluster, federation/domain, licensed/custom-build, and external
  credential integrations in appropriate authorized environments.
- [ ] Verify clean-user Windows installation/uninstallation and full recovery
  from application backups, not just binary builds or local volume reuse.

## Distribution closure

- [ ] Publish the tested 1.1.0 source through a PR and all protected CI checks.
- [ ] Verify post-merge Pages deployment and the real published site.
- [ ] Publish `v1.1.0` and verify all release assets and a fresh installation.

These items are changed to complete only after the corresponding GitHub runs,
release assets, and installer checks actually succeed. Source merges do not
update installer downloads.
