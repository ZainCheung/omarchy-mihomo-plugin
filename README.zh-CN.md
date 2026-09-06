# Mihomo

面向 Omarchy 的 Mihomo 客户端，提供状态栏入口和完整控制面板。普通用户只需要粘贴
订阅、选择节点，然后使用系统代理；需要排查问题时仍可进入高级控制能力。

主要功能包括：远程订阅、本地 YAML 配置档案、档案切换和更新、全局自定义规则、托管
DNS/TUN、源配置/运行时配置/覆盖查看、原子应用与回滚，以及 Mihomo 重启后的当前档案恢复。

托管 TUN 默认使用 `gvisor`，但 **TUN 默认关闭，只有用户主动开启才会接管流量**。仍可选择
`system`、`gvisor` 或 `mixed`；用户明确选择的 `mixed` 会被保留，方便和本机防火墙行为对比。

## 两分钟快速开始

正常流程不要求手写 `config.yaml`、配置 `external-controller`，也不要求理解管理器命令。

1. 如果系统还没有 Mihomo，先安装：

   ```sh
   omarchy pkg add mihomo
   ```

2. 安装并启用插件：

   ```sh
   omarchy plugin add https://github.com/ZainCheung/omarchy-mihomo-plugin.git --enable
   omarchy bar move io.github.ZainCheung.mihomo --section right
   ```

3. 打开 Mihomo 小组件。如果看到提示，点击 **设置 Mihomo**。首次设置会优先采用已经能
   连接的 Mihomo；否则创建插件自己的轻量内核和用户服务。

4. 点击 **添加订阅**，粘贴订阅链接并选择节点。第一个配置档案会自动启用。建议先使用
   **系统代理**；TUN 会保持关闭，直到你主动开启。

如果 TUN 缺少 Linux 权限或和防火墙冲突，面板会保留配置档案并引导你打开网络诊断，而不会
让添加配置档案这一步失败。

## 首次设置做了什么

首次设置会：

- 尽可能复用已经可连接的 Mihomo 内核；
- 在创建新服务前尝试已有的 Mihomo 用户服务；
- 如果没有可用内核，则在 `~/.config/omarchy-mihomo/core/` 创建插件自己的最小启动配置，
  并启动 `omarchy-mihomo.service`；
- 只在用户明确点击首次设置后安装配置管理辅助程序。

这份启动配置只负责让内核稳定运行并提供本地控制器，不开启 TUN，也不依赖 Geo 数据库。插件
不会覆盖用户已有的 Mihomo 配置，也不会静默下载或升级内核二进制。

## 高级：连接已有内核

只有在复用特殊的既有 Mihomo 服务时，才需要手动检查外部控制器，例如：

```yaml
external-controller: 127.0.0.1:9090
```

如果端口或 Secret 不是默认值，可在 `~/.config/omarchy-mihomo/config` 覆盖文件中
指定 `endpoint`、`socket` 或 `secret`。

## 高级：管理器命令

```sh
bin/mihomo-manager profile add --url https://example.test/config.yaml --name 我的订阅
bin/mihomo-manager profile import-current --name 当前配置
bin/mihomo-manager profile import --file /path/to/config.yaml --name 本地配置
bin/mihomo-manager profile list
bin/mihomo-manager profile select <id>
bin/mihomo-manager profile update <id> --via-proxy
bin/mihomo-manager settings patch dns-enable true tun-stack gvisor
bin/mihomo-manager override global get
bin/mihomo-manager override global set --stdin
bin/mihomo-manager rule list
bin/mihomo-manager rule add --domain openai.com --policy proxy
bin/mihomo-manager policy binding get <profile-id>
bin/mihomo-manager policy binding set <profile-id> proxy "代理组"
bin/mihomo-manager reconcile
bin/mihomo-manager doctor
bin/mihomo-manager doctor tun --stack gvisor
```

远程订阅会先下载、解析、编译并通过 `mihomo -t -f` 校验，成功后才替换源配置。全局
自定义规则保存在 `~/.config/omarchy-mihomo/custom-rules.json`，会插入订阅规则之前；
Proxy 规则的代理组选择按配置档案保存在 `profiles/<id>/bindings.json`，Direct/Reject
分别编译为 `DIRECT`/`REJECT`。订阅更新后若原代理组消失，更新会安全失败并保留旧配置，
等待用户重新选择代理组。

托管 DNS/TUN 只覆盖管理器明确拥有的字段，订阅中的其他字段会保留；托管配置的运行时
文件保存在 `~/.config/omarchy-mihomo/runtime/`，包含控制器 Secret 的状态文件权限为 0600。

更完整的实现约束、故障排查和验收用例见
[`docs/implement.md`](docs/implement.md)。
