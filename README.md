# Mihomo

An Omarchy Mihomo client for the bar and shell. Paste a subscription, choose a
node, and use the system proxy; the advanced Mihomo control plane stays
available when you need it.

The client supports remote subscriptions, local YAML imports, profile
switching and updates, global custom rules, managed DNS/TUN settings,
source/runtime/override inspection, atomic apply with rollback, and recovery
after a core restart.

The managed TUN default is `gvisor`, but TUN is **off until the user enables
it**. `system`, `gvisor`, and `mixed` remain available; an explicit `mixed`
choice is preserved so it can be evaluated against local firewall behavior.

## Quick start

The normal path does not require a hand-written `config.yaml`, an
`external-controller` setting, or knowledge of the manager CLI.

1. Install Mihomo if it is not already installed:

   ```sh
   omarchy pkg add mihomo
   ```

2. Install and enable the plugin:

   ```sh
   omarchy plugin add https://github.com/ZainCheung/omarchy-mihomo-plugin.git --enable
   omarchy bar move io.github.ZainCheung.mihomo --section right
   ```

3. Open the Mihomo widget and click **Set up Mihomo** if prompted. On first
   use, the plugin adopts a reachable core or creates its own small bootstrap
   core and user service.

4. Click **Add subscription**, paste the subscription URL, and choose a node.
   The first profile is selected automatically. **System Proxy** is the
   recommended starting point; TUN stays off until you explicitly turn it on.

If TUN needs an extra Linux capability or conflicts with a firewall, the panel
will keep the profile usable and point you to Diagnostics instead of making
profile setup fail.

## What first-time setup does

The one-time setup action:

- reuses an already reachable Mihomo core when possible;
- tries an existing Mihomo user service before creating anything;
- otherwise creates a plugin-owned bootstrap under
  `~/.config/omarchy-mihomo/core/` and starts `omarchy-mihomo.service`;
- installs the profile helper only as part of that explicit setup action.

The bootstrap configuration is intentionally minimal. It enables a local
controller so the client can connect, but it does not enable TUN or depend on
Geo databases. The plugin never overwrites a user's existing Mihomo config or
silently downloads a core binary.

## Install

```sh
omarchy plugin add https://github.com/ZainCheung/omarchy-mihomo-plugin.git --enable
omarchy bar move io.github.ZainCheung.mihomo --section right
```

The plugin installation itself is lightweight. The profile helper is installed
only as part of the explicit first-time setup action (the bundled
`bin/install-manager`), or can be provided during development with
`OMARCHY_MIHOMO_MANAGER_BIN=/path/to/binary`.

## Manager CLI

```sh
bin/mihomo-manager status
bin/mihomo-manager profile add --url https://example.test/config.yaml --name MySubscription
bin/mihomo-manager profile import-current --name "Existing config"
bin/mihomo-manager profile import --file /path/to/config.yaml --name "Local file"
bin/mihomo-manager profile list
bin/mihomo-manager profile select <id>
bin/mihomo-manager profile update <id> [--via-proxy]
bin/mihomo-manager settings patch dns-enable true tun-stack gvisor
bin/mihomo-manager override global get
bin/mihomo-manager override global set --stdin
bin/mihomo-manager rule list
bin/mihomo-manager rule add --domain openai.com --policy proxy
bin/mihomo-manager policy binding get <profile-id>
bin/mihomo-manager policy binding set <profile-id> proxy "Proxy group"
bin/mihomo-manager reconcile
bin/mihomo-manager doctor
bin/mihomo-manager doctor tun --stack gvisor
```

All manager mutations use `~/.config/omarchy-mihomo` (or
`OMARCHY_MIHOMO_HOME`), private directories/files, an exclusive `flock`,
temporary files, fsync, and atomic rename. Subscription metadata is kept in
`profiles/<uuid>/meta.json`; `source.yaml` and `override.yaml` are never
round-trip rewritten. Runtime files are in `runtime/`.

The compiler's V1 merge rules are recursive map merge, scalar replacement,
whole-array replacement, and `null` deletion. Global custom rules are stored in
`custom-rules.json` and prepended to the subscription rules; they never replace
the subscription's rule array. Proxy rules use a binding stored in
`profiles/<uuid>/bindings.json`, while Direct and Reject compile to `DIRECT` and
`REJECT` without a binding. A changed subscription fails safely with
`binding_required` when its saved proxy group is no longer available.

DNS and TUN default to Managed; managed TUN uses the `gvisor` stack by default
and remains disabled until the user enables it. Managed DNS/TUN overlay only
the fields exposed by the manager and preserve other source/override fields;
set either management mode to `inherit` to leave that section untouched.
Managed profiles preserve the running core's `external-controller`, Unix
controller, `secret`, and `external-ui` fields.

## Advanced control

The plugin still exposes the full panel for users who need it: proxy groups,
connections, rules, diagnostics, source/runtime/override inspection, and the
manager CLI. These are implementation and troubleshooting surfaces, not
required installation steps.

For existing custom controllers, unusual service layouts, or Geo resource
failures, see the troubleshooting and implementation notes in
[`docs/implement.md`](docs/implement.md).

## Advanced modes

**Raw Config Mode** is the legacy-compatible mode. Until a profile is added or
the current config is imported, the plugin reads the running core and keeps the
existing runtime TUN and system-proxy controls.

**Managed Profile Mode** treats a subscription as **source configuration, not as
the final Mihomo runtime configuration**. The manager stores source YAML and
small overrides separately, compiles them into a runtime YAML, validates it with
`mihomo -t -f`, then applies it transactionally.

```text
Source Config → Global Override → Profile Override → Custom Rules
              → Managed DNS/TUN → Protected controller fields
              → validation → runtime/current.yaml
```

## Development

```sh
./deploy
cd manager && go test ./...
./tests/integration.sh
./tests/bootstrap.sh
```

The repository is MIT licensed. It does not copy Clash Verge Rev source code;
its product behavior is only a reference.
