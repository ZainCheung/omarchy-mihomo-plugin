# Mihomo

这是一个面向 Omarchy 的 Mihomo 状态栏插件和控制面板。它直接连接正在运行的
Mihomo 外部控制器，不负责启动或安装 Mihomo 进程。

主要功能：

- 代理模式、节点、系统代理和 TUN 流量接管
- 远程订阅与本地 YAML 配置档案管理
- 配置档案切换、更新、重命名、删除和通过代理更新
- Source Config、Runtime Config、全局覆盖和档案覆盖查看
- 托管 DNS/TUN 设置，编译后校验再重载
- 应用失败自动回滚、内核重启后恢复当前档案
- 连接、规则、DNS/TUN、权限和代理连通性诊断

## 安装

```sh
omarchy plugin add https://github.com/ZainCheung/omarchy-mihomo-plugin.git --enable
omarchy bar move io.github.ZainCheung.mihomo --section right
```

插件不会自动下载 Mihomo 或执行 sudo。首次使用配置档案功能时，在「配置档案」页面
明确点击安装配置管理器即可。

## 连接内核

请在 Mihomo 配置中启用外部控制器，例如：

```yaml
external-controller: 127.0.0.1:9090
```

如果端口或 Secret 不是默认值，可在 `~/.config/omarchy-mihomo/config` 覆盖文件中
指定 `endpoint`、`socket` 或 `secret`。

## 管理器命令

```sh
bin/mihomo-manager profile add --url https://example.test/config.yaml --name 我的订阅
bin/mihomo-manager profile import-current --name 当前配置
bin/mihomo-manager profile import --file /path/to/config.yaml --name 本地配置
bin/mihomo-manager profile list
bin/mihomo-manager profile select <id>
bin/mihomo-manager profile update <id> --via-proxy
bin/mihomo-manager doctor
```

远程订阅会先下载、解析、编译并通过 `mihomo -t -f` 校验，成功后才替换源配置。
托管 DNS/TUN 只覆盖管理器明确拥有的字段，订阅中的其他字段会保留；TUN 默认使用
`gvisor`，但用户明确选择的 `system` 或 `mixed` 会原样保留。托管配置的运行时文件保存在
`~/.config/omarchy-mihomo/runtime/`，包含控制器 Secret 的状态文件权限为 0600。

更完整的实现约束、合并顺序和验收用例见
[`docs/implement.md`](docs/implement.md)。
