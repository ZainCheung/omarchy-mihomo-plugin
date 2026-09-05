# Mihomo

Omarchy bar plugin and native Mihomo client/control plane. It keeps the existing
Home / Proxies / Config / Connections / Rules panel and adds a Go profile manager.

The managed TUN default is `gvisor`; legacy `mixed` settings are migrated when
loaded or compiled because `mixed` is unreliable on some Linux setups.

## Modes

**Raw Config Mode** is the legacy-compatible mode. Until a profile is added or
the current config is imported, the plugin reads the running core and keeps the
existing runtime TUN and system-proxy controls.

**Managed Profile Mode** treats a subscription as **source configuration, not as
the final Mihomo runtime configuration**. The manager stores source YAML and
small overrides separately, compiles them into a runtime YAML, validates it with
`mihomo -t -f`, then applies it transactionally.

```text
Source Config → Global Override → Profile Override → Managed DNS/TUN
              → Protected controller fields → validation → runtime/current.yaml
```

## Install

```sh
omarchy plugin add https://github.com/lijiawei0305-pixel/omarchy-mihomo-plugin.git --enable
omarchy bar move io.github.lijiawei0305-pixel.mihomo --section right
```

Installation never downloads a helper or uses sudo. Profile Manager is installed
only after an explicit user action (the bundled `bin/install-manager`), or can be
provided during development with `OMARCHY_MIHOMO_MANAGER_BIN=/path/to/binary`.

## Manager CLI

```sh
bin/mihomo-manager status
bin/mihomo-manager profile add --url https://example.test/config.yaml --name MySubscription
bin/mihomo-manager profile import-current --name "Existing config"
bin/mihomo-manager profile list
bin/mihomo-manager profile select <id>
bin/mihomo-manager profile update <id> [--via-proxy]
bin/mihomo-manager reconcile
bin/mihomo-manager doctor
```

All manager mutations use `~/.config/omarchy-mihomo` (or
`OMARCHY_MIHOMO_HOME`), private directories/files, an exclusive `flock`,
temporary files, fsync, and atomic rename. Subscription metadata is kept in
`profiles/<uuid>/meta.json`; `source.yaml` and `override.yaml` are never
round-trip rewritten. Runtime files are in `runtime/`.

The compiler's V1 merge rules are recursive map merge, scalar replacement,
whole-array replacement, and `null` deletion. DNS and TUN default to Managed;
managed TUN uses the `gvisor` stack by default. Existing `mixed` TUN settings
are normalized to `gvisor`; set either management mode to `inherit` to preserve
source/override values.
Managed profiles preserve the running core's `external-controller`, Unix
controller, `secret`, and `external-ui` fields.

## Existing panel

The plugin still talks to the running Mihomo external controller through
`bin/mihomo-ctl`. It does not start or install Mihomo. `mihomo-ctl` owns REST,
traffic, proxy, connection, rule, reload, and Linux system-proxy operations;
`mihomo-manager` owns persistent profiles, compilation, validation, apply, and
rollback. See the original panel pages for live core status and controls.

## Development

```sh
./deploy
cd manager && go test ./...
```

The repository is MIT licensed. It does not copy Clash Verge Rev source code;
its product behavior is only a reference.
