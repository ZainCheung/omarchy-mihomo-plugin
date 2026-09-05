
# 完整实施指令

## 项目目标

将它从目前的：

> Omarchy 上用于控制 standalone Mihomo Core 的轻量面板

升级为：

> Omarchy 原生的完整 Mihomo Client / Control Plane。

目标体验参考 Clash Verge Rev，但不要复制 Clash Verge Rev 的 GPL 源代码。当前插件是 MIT，而 Clash Verge Rev 是 GPL-3.0，因此只能研究其产品能力、架构和行为，然后独立实现。([GitHub][3])

最终需要解决：

1. URL 添加订阅。
2. 多 Profile 管理。
3. Profile 一键切换。
4. 订阅更新。
5. 自动更新。
6. Update via proxy。
7. Omarchy/Linux 友好的完整 TUN 配置。
8. 完整 DNS 配置。
9. Source Config 与 Runtime Config 分离。
10. Global / Profile Override。
11. 配置编译。
12. Mihomo config validation。
13. 原子切换。
14. Apply 失败自动 rollback。
15. Core 重启后自动恢复当前 Profile。
16. Runtime / Source / Override 查看。
17. 网络诊断。
18. 保留当前 Proxies / Connections / Rules / System Proxy 能力。

---

# 一、先理解当前代码，禁止直接重写

开始修改前，先完整阅读：

```text
README.md
manifest.json
Service.qml
MihomoPanel.qml
HomePage.qml
ConfigPage.qml
ProxiesPage.qml
ConnectionsPage.qml
RulesPage.qml
I18n.qml
bin/mihomo-ctl
deploy
```

当前架构必须保留：

```text
MihomoPanel.qml
      │
      ▼
Service.qml
      │
      ▼
bin/mihomo-ctl
      │
      ▼
Mihomo External Controller
```

`bin/mihomo-ctl` 继续负责：

```text
controller endpoint discovery
GET/PUT/PATCH/DELETE REST API
system proxy
traffic/config/proxy operations
```

不要把 Profile/YAML 编译逻辑继续塞进 `mihomo-ctl`。

这个文件当前的设计本来就是 dependency-free Bash controller client，而且只使用 shallow YAML parsing 来读取少量字段。

新增一个明确独立的配置管理层。

---

# 二、新架构

目标架构：

```text
                  Omarchy Shell
                       │
                MihomoPanel.qml
                       │
                 Service.qml
                /           \
               /             \
      mihomo-ctl         mihomo-manager
          │                    │
          │                    ├── Profile Manager
          │                    ├── Subscription Fetcher
          │                    ├── Config Compiler
          │                    ├── Validator
          │                    ├── Runtime Manager
          │                    ├── Rollback Manager
          │                    └── Doctor
          │
          ▼
    Mihomo REST API
          │
          ▼
      Mihomo Core
```

原则：

```text
mihomo-ctl
= Runtime / API control

mihomo-manager
= Persistent configuration management
```

不要混合这两层。

---

# 三、mihomo-manager 使用 Go 实现

不要用 Bash 拼 YAML。

建立：

```text
manager/
├── go.mod
├── cmd/
│   └── omarchy-mihomo-manager/
│       └── main.go
│
└── internal/
    ├── profile/
    ├── fetcher/
    ├── config/
    ├── runtime/
    ├── validator/
    ├── core/
    ├── store/
    └── doctor/
```

使用：

```text
Go
yaml.v3
standard library
```

尽量只引入一个 YAML dependency。

原因：

* 必须支持真正的 YAML parsing。
* 必须 deep merge。
* 必须处理 arrays/maps/scalars。
* 必须支持事务式切换。
* 必须安全处理 URL。
* Bash 不适合继续承担这一层复杂度。

Source YAML 永远不进行 round-trip rewrite，因此无需保留其 comments。

---

# 四、配置目录

继续兼容目前：

```text
~/.config/omarchy-mihomo/config
~/.config/omarchy-mihomo/ui
```

新增：

```text
~/.config/omarchy-mihomo/

├── config
├── ui
│
├── settings.json
│
├── profiles/
│   ├── index.json
│   │
│   ├── <profile-id>/
│   │   ├── meta.json
│   │   ├── source.yaml
│   │   └── override.yaml
│   │
│   └── <profile-id>/
│       ├── meta.json
│       ├── source.yaml
│       └── override.yaml
│
├── overrides/
│   └── global.yaml
│
├── runtime/
│   ├── current.yaml
│   ├── previous.yaml
│   ├── candidate.yaml
│   └── state.json
│
└── locks/
    └── manager.lock
```

所有包含 subscription URL/token 的文件：

```text
0600
```

目录：

```text
0700
```

日志绝对不允许输出完整 subscription URL。

例如：

```text
https://example.com/sub?token=abcdef123456
```

显示成：

```text
https://example.com/sub?token=••••••••
```

---

# 五、Profile 数据模型

`meta.json`：

```json
{
  "schemaVersion": 1,
  "id": "uuid",
  "name": "My Subscription",
  "type": "remote",
  "url": "https://example.com/sub",
  "updateIntervalSec": 21600,
  "updateViaProxy": false,
  "fetchUserAgent": "",
  "createdAt": "",
  "lastUpdatedAt": "",
  "lastSuccessAt": "",
  "etag": "",
  "lastModified": "",
  "subscriptionInfo": {
    "upload": 0,
    "download": 0,
    "total": 0,
    "expire": 0
  }
}
```

支持：

```text
type = remote
type = local
```

Remote：

```text
URL subscription
```

Local：

```text
导入当前 config
本地 YAML
```

Profile ID 使用随机 UUID，不允许使用 name 当文件名。

---

# 六、最核心的 Config Compiler

必须建立这个 pipeline：

```text
source.yaml
     │
     ▼
Global Override
     │
     ▼
Profile Override
     │
     ▼
Managed Omarchy Enhancement
     │
     ▼
Protected Runtime Fields
     │
     ▼
candidate.yaml
     │
     ▼
mihomo -t -f
     │
     ▼
runtime/current.yaml
```

核心概念必须贯彻整个项目：

```text
Source Config != Runtime Config
```

用户下载的机场订阅绝不能直接成为 runtime config。

---

# 七、Merge 规则

V1 定义固定、可预测的规则。

对于 YAML map：

```text
recursive deep merge
```

对于 scalar：

```text
后层覆盖前层
```

对于 array：

```text
后层整体替换
```

不要第一版实现复杂 array append/prepend DSL。

Override 中：

```yaml
some-key: null
```

定义为：

```text
删除 some-key
```

必须给这个行为写 unit test。

Merge 顺序：

```text
1. Source
2. Global Override
3. Profile Override
4. Managed Settings
5. Protected Runtime Settings
```

其中 Managed Settings 是最高级的用户 GUI 设置，因此开启 Managed DNS/TUN 时应覆盖订阅对应的已建模字段；未被管理器建模的 Mihomo 字段必须保留，不能因为版本落后而丢失。

---

# 八、Managed / Inherit 模式

DNS 与 TUN 都不能简单粗暴覆盖机场配置。

设计：

```text
DNS management:
- Managed
- Inherit

TUN management:
- Managed
- Inherit
```

`inherit`：

```text
完全使用订阅 / override 中的数据
```

`managed`：

```text
由 Omarchy Mihomo 覆盖已建模字段，并保留 source / override 中的其他字段
```

Managed DNS/TUN 使用 known-fields overlay，而不是替换整个 map：

```text
source.dns / source.tun
        ↓
manager-owned field patch
        ↓
保留未知字段的 runtime config
```

默认推荐：

```text
DNS = Managed
TUN = Managed
```

但必须允许高级用户切回 Inherit。

---

# 九、TUN Preset

当前 `Service.qml` 已经动态补：

```yaml
auto-route: true
auto-detect-interface: true
stack: gvisor
dns-hijack:
  - any:53
```

但新的 managed mode 不应继续依赖 runtime PATCH。

Managed TUN 的默认栈是 `gvisor`，但允许 `gvisor`、`system`、`mixed` 三个 Mihomo 值。
空值才使用默认值；用户显式选择 `mixed` 时不得在读取、保存或编译时静默改写为
`gvisor`。Doctor 可以提示 system/mixed 与防火墙的兼容性风险，但不替用户迁移配置。

改为生成持久 runtime：

```yaml
tun:
  enable: true
  stack: gvisor
  auto-route: true
  auto-detect-interface: true
  strict-route: false
  dns-hijack:
    - any:53
    - tcp://any:53
```

第一版不要默认加入：

```text
auto-redirect
iptables/nftables 自定义规则
手工 route table
```

避免在不同 Linux 环境制造新的网络问题。

---

# 十、DNS Preset

默认 managed DNS：

```yaml
dns:
  enable: true
  ipv6: false

  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16

  default-nameserver:
    - 223.5.5.5
    - 1.1.1.1

  nameserver:
    - https://dns.alidns.com/dns-query
    - https://1.1.1.1/dns-query

  proxy-server-nameserver:
    - https://dns.alidns.com/dns-query
    - https://1.1.1.1/dns-query

  fake-ip-filter:
    - "*.lan"
    - "*.local"
    - "localhost"
```

不要硬编码更多 app-specific filter。

UI 后续允许修改：

```text
DNS mode
IPv6
fake-ip range
nameserver
bootstrap nameserver
proxy-server-nameserver
```

---

# 十一、Protected Runtime Fields

远程订阅不能获得控制插件自身 controller 的能力。

以下字段在 Managed Profile 中属于 protected：

```text
external-controller
external-controller-unix
secret
external-ui
```

第一次启用 Profile Manager 时：

1. 获取当前 running core endpoint。
2. 找到 running config。
3. 保存当前 controller 信息。
4. 编译所有 Profile 时强制保持这个 controller。

如果当前是：

```yaml
external-controller: 127.0.0.1:9090
secret: xxx
```

切换任何 Profile 后都必须仍然存在。

禁止远程 subscription 修改 controller 地址或 secret。

---

# 十二、扩展 mihomo-ctl

不要重构它已有功能。

只新增：

```bash
mihomo-ctl coreinfo
```

JSON：

```json
{
  "pid": 1234,
  "exe": "/usr/bin/mihomo",
  "configPath": "/etc/mihomo/config.yaml",
  "configDir": "/etc/mihomo",
  "controllerTransport": "tcp",
  "controllerTarget": "127.0.0.1:9090"
}
```

不要在输出中泄露 secret。

Manager 使用它获取：

```text
mihomo binary
running config
controller
```

---

# 十三、Config Validation

任何 config 在 apply 前：

```bash
mihomo -t -f candidate.yaml
```

Mihomo 当前确实支持该验证方式。([GitHub][4])

顺序严格为：

```text
Compile
↓
Parse
↓
mihomo -t
↓
Apply
```

没有通过 validation：

```text
绝对不能触碰 current profile
绝对不能触碰 current runtime
绝对不能 reload core
```

将 Mihomo stderr 转成结构化错误给 QML：

```json
{
  "ok": false,
  "stage": "validate",
  "error": "..."
}
```

---

# 十四、Profile 切换必须事务化

实现：

```text
select Profile B
      │
      ▼
compile B
      │
      ▼
candidate.yaml
      │
      ▼
validate
      │
   failure
      └──────→ stop
      │
   success
      ▼
backup current
      │
      ▼
apply candidate
      │
      ▼
check controller
      │
   success
      ▼
promote candidate → current
      │
      ▼
activeProfile = B
```

失败：

```text
重新 apply previous.yaml
activeProfile 不改变
```

使用：

```text
flock
temporary file
fsync
atomic rename
```

防止：

```text
UI switch
auto update
manual update
```

同时写配置。

---

# 十五、Apply

继续复用 Mihomo API：

```text
PUT /configs?force=true
```

传入：

```json
{
  "path": "",
  "payload": "<compiled YAML>"
}
```

The manager sends the compiled YAML in `payload` because Mihomo rejects a
private plugin-store path that is outside its configured safe paths. The
request is `PUT /configs?force=true`; `force=true` belongs in the query string,
not in the JSON body.

成功后检查：

```text
GET /version
GET /configs
```

如果 controller 不可达：

```text
立即 rollback previous.yaml
```

---

# 十六、Core restart reconciliation

第一版不要接管 Mihomo service lifecycle。

仍保持 upstream：

```text
plugin controls external core
```

但是解决 Profile 在 core restart 后丢失的问题。

Service.qml 需要检测：

```text
connected:
true → false → true
```

当出现 reconnect：

```text
mihomo-manager reconcile
```

如果存在 active profile：

```text
重新 apply runtime/current.yaml
```

Service 第一次启动也执行一次：

```text
reconcile
```

因此：

```text
system reboot
↓
mihomo 从 /etc/mihomo/config.yaml 启动
↓
Omarchy shell 启动
↓
Service.qml
↓
reconcile
↓
恢复用户选中的 Profile runtime
```

第一阶段不用修改 systemd mihomo unit。

---

# 十七、Subscription Fetcher

Remote profile 支持：

```text
HTTP
HTTPS
redirect <= 5
timeout = 30 sec
response max = 16 MiB
ETag
If-Modified-Since
```

默认：

```text
direct fetch
```

支持：

```text
Update via Proxy
```

Proxy 更新通过当前 Mihomo：

```text
http://127.0.0.1:<mixed-port>
```

不要自动 fallback。

显式提供：

```text
Update
Update via proxy
```

以后再考虑自动 fallback。

更新流程：

```text
download to temp
↓
parse YAML
↓
compile temp source
↓
validate
↓
如果 active → apply
↓
成功后才替换 source.yaml
```

如果 remote 返回坏配置：

```text
当前网络必须完全不受影响
```

---

# 十八、Profile Manager CLI

实现稳定 JSON API：

```bash
omarchy-mihomo-manager status

omarchy-mihomo-manager profile list
omarchy-mihomo-manager profile get <id>

omarchy-mihomo-manager profile add \
  --url <url> \
  --name <name>

omarchy-mihomo-manager profile import-current \
  --name <name>

omarchy-mihomo-manager profile import \
  --file <path> \
  --name <name>

omarchy-mihomo-manager profile update <id>
omarchy-mihomo-manager profile update <id> --via-proxy

omarchy-mihomo-manager profile select <id>

omarchy-mihomo-manager profile delete <id>

omarchy-mihomo-manager profile rename <id> <name>

omarchy-mihomo-manager profile source <id>
omarchy-mihomo-manager profile runtime <id>

omarchy-mihomo-manager config compile <id>
omarchy-mihomo-manager config apply <id>
omarchy-mihomo-manager config rollback

omarchy-mihomo-manager reconcile

omarchy-mihomo-manager settings get
omarchy-mihomo-manager settings set ...

omarchy-mihomo-manager doctor
```

所有供 QML 调用的命令：

```text
stdout = JSON
stderr = diagnostic only
exit code != 0 on failure
```

---

# 十九、Profiles UI

新增：

```text
ProfilesPage.qml
ProfileDetail.qml
AddProfileDialog.qml
```

导航变成：

```text
Home
Profiles
Proxies
Config
Connections
Rules
```

更新 keyboard：

```text
1 Home
2 Profiles
3 Proxies
4 Config
5 Connections
6 Rules
```

保留：

```text
h/l
left/right
j/k
r
```

---

# 二十、Profiles 页面 UI

大致：

```text
Profiles

                         + Add

● My Subscription
  Remote
  Updated 12 min ago
  235 GB / 500 GB
                          ↻   ⋯

○ Backup
  Remote
  Updated yesterday
                          ↻   ⋯

○ Local Config
  Local
```

Active profile：

```text
●
```

点击 Profile 本身：

```text
switch
```

不要让点击三点菜单导致切换。

三点菜单：

```text
Update
Update via Proxy
Rename
Edit URL
Edit Override
View Source
View Runtime
Delete
```

---

# 二十一、Add Profile

支持：

```text
URL
Name
Update interval
```

Update interval：

```text
Disabled
1 hour
6 hours
12 hours
24 hours
```

添加时：

```text
先 fetch
先 validate
成功后才加入列表
```

不能先创建一个坏 Profile。

Empty state：

```text
No profiles yet

[ Add from URL ]

[ Import Current Config ]
```

`Import Current Config` 很重要。

用户当前已经有：

```text
/etc/mihomo/config.yaml
```

可以立即迁移成 local profile。

---

# 二十二、Config 页面改造

先不要删除现有 ConfigPage。

在原页面加入：

```text
Configuration

Active Profile
My Subscription

Source Config
[ View ]

Runtime Config
[ View ]

Global Override
[ Edit ]

Profile Override
[ Edit ]

Network Management
DNS    Managed
TUN    Managed

[ Recompile & Reload ]
```

现有：

```text
config path
size
mtime
providers
reload
open config
```

都保留。

---

# 二十三、Network Settings

Config 或 Settings 中增加：

```text
TUN

[✓] Managed
[✓] Enabled

Stack
GVisor

[✓] Auto Route
[✓] Auto Detect Interface
[✓] DNS Hijack


DNS

[✓] Managed
[✓] Enabled

Mode
Fake-IP

IPv6
Off

Fake IP Range
198.18.0.1/16
```

修改这些字段：

```text
settings.json
↓
compile active profile
↓
validate
↓
apply
```

Profile Managed 模式下，禁止再只做：

```text
PATCH /configs
```

否则重启以后又会消失。

---

# 二十四、Legacy Mode

这是兼容性关键。

如果：

```text
activeProfile == null
```

那么插件行为必须与 upstream 0.4.0 基本一致。

也就是：

```text
继续读取 running config
继续 runtime TUN toggle
继续现有 ConfigPage
```

不要用户升级插件之后立刻接管：

```text
/etc/mihomo/config.yaml
```

只有用户：

```text
Add Profile
Import Current Config
```

以后才进入 Managed Profile Mode。

---

# 二十五、Auto Update

因为 manifest 已经有：

```text
service
```

第一版直接在 `Service.qml` 增加后台 Timer。

例如：

```text
15 minutes
```

执行：

```bash
omarchy-mihomo-manager profile update-due
```

Manager 自己判断：

```text
lastUpdated + updateInterval
```

不要 QML 自己计算每个 Profile。

更新 inactive profile：

```text
只更新 source
```

更新 active profile：

```text
fetch
compile
validate
apply
promote
```

失败：

```text
保持旧配置
```

不要弹频繁 notification。

只在 Profiles 页面显示：

```text
Last update failed
```

---

# 二十六、Doctor

新增：

```bash
omarchy-mihomo-manager doctor
```

检测：

```text
Mihomo process
Mihomo version
Core executable
Controller API
Current config
Active profile
Runtime config
Config validation
TUN enabled
TUN stack
TUN interface
cap_net_admin
DNS enabled
DNS mode
systemd-resolved
/etc/resolv.conf
resolvectl
HTTP proxy port
DNS query
Proxy connectivity
```

返回：

```json
{
  "checks": [
    {
      "id": "controller",
      "status": "ok",
      "message": "127.0.0.1:9090"
    },
    {
      "id": "tunCapability",
      "status": "error",
      "message": "mihomo is missing cap_net_admin"
    }
  ]
}
```

以后这种：

```text
Codex OAuth
Could not resolve host
```

用户打开 Home → Diagnostics 就应该能一眼看到：

```text
DNS
ERROR

TUN
WARNING
```

而不用终端排查半天。

---

# 二十七、不要自动 sudo

插件不能：

```text
sudo setcap
sudo pacman
sudo 修改 resolv.conf
sudo 写 /etc
```

如果检测到缺少 `cap_net_admin`：

UI 显示：

```text
TUN requires cap_net_admin

Run:

sudo setcap cap_net_admin,cap_net_raw=+ep /path/to/mihomo
```

提供：

```text
Copy command
Open terminal
```

但不要自己执行 sudo。

---

# 二十八、Service.qml 的修改原则

当前 `Service.qml` 已经接近 1000 行，不要无限膨胀。

新增的 profile state 控制在：

```text
runner = mihomo-ctl
managerRunner = mihomo-manager

profiles
activeProfile
profileLoading
profileMutating
profileError
```

Profile 网络操作使用独立 Process queue：

```text
managerActionQueue
```

不要复用现有：

```text
actionQueue
```

因为当前 actionQueue 用于：

```text
node switch
mode switch
system proxy
reload
```

Subscription update 最长可能 30 秒，不能让一次订阅更新把节点切换卡住。

---

# 二十九、推荐文件结构

完成后大致：

```text
omarchy-mihomo-plugin/

├── manager/
│   ├── go.mod
│   ├── cmd/
│   └── internal/
│
├── bin/
│   ├── mihomo-ctl
│   ├── mihomo-manager
│   └── install-manager
│
├── ProfilesPage.qml
├── ProfileDetail.qml
├── AddProfileDialog.qml
│
├── HomePage.qml
├── ConfigPage.qml
├── ProxiesPage.qml
├── ConnectionsPage.qml
├── RulesPage.qml
│
├── Service.qml
├── MihomoPanel.qml
├── I18n.qml
│
├── tests/
│   └── fixtures/
│       ├── subscription-basic.yaml
│       ├── subscription-with-dns.yaml
│       ├── invalid.yaml
│       └── override.yaml
│
└── .github/
    └── workflows/
```

---

# 三十、Go helper distribution

Omarchy plugin installation不会执行 build/install hook，所以不要假设用户有 Go。

开发阶段：

```bash
./deploy
```

如果检测到 Go：

```text
build manager
copy to dev data dir
```

正式发行：

GitHub Actions 构建：

```text
linux-amd64
linux-arm64
```

生成：

```text
omarchy-mihomo-manager-linux-amd64
omarchy-mihomo-manager-linux-arm64
SHA256SUMS
```

`bin/install-manager`：

1. 判断 arch。
2. 下载对应 release。
3. 校验 SHA256。
4. 安装到：

```text
~/.local/share/omarchy-mihomo/bin/
```

绝对禁止 silently download。

Profiles 第一次打开时如果 manager 不存在：

```text
Profile Manager is not installed

[ Install ]
```

用户点击后才下载安装。

开发模式允许：

```text
OMARCHY_MIHOMO_MANAGER_BIN=/path/to/dev/binary
```

---

# 三十一、Tests

必须写 Go unit tests。

至少覆盖：

```text
deep merge map
scalar override
array replacement
null delete

managed DNS
inherit DNS

managed TUN
inherit TUN
managed DNS/TUN preserve unknown fields
explicit mixed TUN stack

protected controller fields

URL redaction

profile add
profile rename
profile delete

invalid update doesn't replace old source

inactive profile update

active profile update

profile switch

apply failure rollback

ETag 304

redirect / DNS resolution SSRF protection

settings patch applies a burst of UI edits once

atomic file write

concurrent lock
```

最关键 fixture：

`subscription-basic.yaml`

故意没有：

```yaml
dns:
tun:
```

编译后必须出现：

```yaml
dns:
tun:
```

这正是此次项目的核心 regression test。

---

# 三十二、Integration Test

通过 env 注入：

```text
MIHOMO_CTL
MIHOMO_BIN
OMARCHY_MIHOMO_HOME
```

使用 fake binaries。

测试：

```text
Profile A
↓
select
↓
runtime A

Profile B
↓
select
↓
runtime B
```

模拟：

```text
mihomo-ctl PUT failure
```

验证：

```text
activeProfile 仍然 A
current.yaml 仍然 A
```

---

# 三十三、手工验收

完成后必须在真实 Omarchy 上验证：

### Case 1

使用当前这份：

```text
不包含 dns:
不包含 tun:
```

的订阅。

URL 导入。

Runtime config 应自动包含：

```text
DNS
TUN
```

开启 TUN 后：

```bash
getent ahostsv4 auth.openai.com
```

能够正常解析。

### Case 2

```text
Profile A
→ Profile B
→ Profile A
```

节点和规则正确切换。

### Case 3

订阅服务器返回 invalid YAML。

结果：

```text
当前网络完全不断
当前 Profile 不变
```

### Case 4

关闭 Mihomo → 重启 Mihomo。

Service reconnect 后：

```text
active Profile 自动恢复
```

### Case 5

System Proxy ON。

切换到 mixed-port 不同的 Profile。

现有 `mihomo-ctl` 逻辑应自动刷新 system proxy port，不产生 regression。当前代码已经会把 proxy 环境写入 `environment.d` 和 systemd user environment，这块不要破坏。

---

# 三十四、开发阶段

不要一次做一个巨大 commit。

按以下阶段提交。

### Phase 1 — Manager Foundation

```text
Go manager
storage
profile model
URL fetch
import current
list/add/delete/update
unit tests
```

此阶段不改大 UI。

### Phase 2 — Config Compiler

```text
deep merge
override
managed DNS
managed TUN
protected fields
mihomo -t validation
runtime
transaction
rollback
reconcile
```

这一阶段结束时，CLI 必须已经能：

```bash
manager profile add
manager profile select
```

并真的成功切换 Mihomo。

### Phase 3 — Profiles UI

```text
ProfilesPage
AddProfile
switch
update
delete
rename
active state
```

### Phase 4 — Network Settings

```text
DNS Managed/Inherited
TUN Managed/Inherited
network settings
recompile/apply
```

### Phase 5 — Background + Diagnostics

```text
auto update
update via proxy
doctor
reconnect reconcile
```

### Phase 6 — Distribution

```text
PR / branch push checks
go test ./...
integration.sh
plugin manifest validation
GitHub Actions
amd64
arm64
checksums
manager installer
README
```

---

# 三十五、明确 Non-goals

第一版不要做：

```text
WebDAV
JavaScript enhancement scripts
visual rule editor
visual proxy group editor
managed Mihomo core installer
Mihomo auto upgrade
sudo helper
Windows
macOS
subscription converter
URI/node import
mobile remote control
```

先把：

```text
Profiles
DNS
TUN
Compiler
Validation
Rollback
```

做到稳定。

---

# 三十六、README 需要重写相关章节

最终 README 应明确解释：

```text
Raw Config Mode
Managed Profile Mode
```

以及：

```text
Source Config
Override
Runtime Config
```

三者区别。

必须明确：

> A subscription is treated as source configuration, not as the final Mihomo runtime configuration.

并说明：

```text
Managed DNS/TUN
```

会在 runtime compiler 中生成配置。

---

# 三十七、兼容性原则

所有现有能力必须继续工作：

```text
Home
Proxies
node switch
latency
Rule / Global / Direct
System Proxy
Connections
close connection
Rules
providers
traffic
memory
Chinese / English
IPC
keyboard navigation
```

禁止为了 Profile Manager 大规模重构已有稳定页面。

---

# 三十八、代码质量要求

重点遵守：

```text
QML only UI/state orchestration
Go owns persistent config logic
Bash owns controller/system-proxy glue
```

不要出现：

```text
QML 解析 YAML
QML 手工拼 config
Bash regex merge YAML
Service.qml 直接 curl subscription
```

所有 manager mutation 必须：

```text
idempotent
transactional
atomic
recoverable
```

---

# 三十九、最终提交报告

开发完成后不要只说“完成”。

输出：

```text
1. 改了哪些文件
2. 新架构说明
3. Profile storage schema
4. merge precedence
5. TUN/DNS runtime 示例
6. rollback 工作流程
7. 测试结果
8. omarchy plugin validate 结果
9. Go test 结果
10. 实机验证结果
11. 尚未完成的 Phase
12. 已知风险
```

并给出：

```bash
git diff --stat
git log --oneline
```

---

## 我建议额外强调给 Codex 的一条

**不要把目标理解成“给现有插件加一个订阅输入框”。**

真正需要实现的是：

```text
Subscription
      ↓
Source Profile
      ↓
Config Compiler
      ↓
Enhancements
      ↓
Validation
      ↓
Runtime Config
      ↓
Transactional Apply
      ↓
Mihomo
```

Profile UI 只是这套系统的入口。

如果没有这个 pipeline，即使能 URL 下载订阅，本质上还是会重新遇到今天这种：

```text
Clash Verge Rev TUN ✅
raw Mihomo TUN ❌
DNS broken
```

的问题。

---

我会建议你让 Codex **先完成 Phase 1 + Phase 2，不急着做 UI**。因为一旦 CLI 层能做到：

```bash
omarchy-mihomo-manager profile add ...
omarchy-mihomo-manager profile select ...
```

并让你当前这份“不包含 DNS/TUN 的订阅”在 Omarchy 上成功 TUN + DNS，整个项目最难、也最有价值的部分就已经验证完了。之后 QML 基本只是把这些能力产品化。

---

# 四十、文案与元数据质量验收

文案检查报告纳入实施计划，与功能验收同等对待。每次新增 UI、CLI 或脚本输出时，
同时检查以下约束：

```text
Profile = 配置档案
Remote profile = 远程订阅
Local profile = 本地导入
只读状态使用「已启用 / 已停用」
可执行动作使用「开启 / 关闭」
TUN 统一写作「TUN（虚拟网卡）」
```

具体要求：

1. 中英文 key 必须一一对应；页面不得硬编码面向用户的中英文句子。
2. 中文提示使用完整书面句，统一中文引号为「」；英文提示补齐主语、谓语和介词。
3. 数量文案必须处理单复数，避免 `1 profiles`、`1 rules` 一类输出。
4. URL、Secret、token 和命令错误不能出现在普通列表或日志中；显式查看 URL 才返回原值。
5. README、manifest、模块 ID、安装脚本、Go module 和 GitHub Release 地址必须使用同一仓库身份。
6. README 同时提供英文和简体中文快速上手说明，并明确说明插件不启动 Mihomo、不自动执行 sudo。

文案改动的回归检查：

```text
英文界面：Profile / Remote subscription / Local import / Enabled / Disabled
中文界面：配置档案 / 远程订阅 / 本地导入 / 已启用 / 已停用
未连接、更新失败、校验失败、重载失败均有明确可操作的提示
```

[1]: https://github.com/ZainCheung/omarchy-mihomo-plugin "GitHub - ZainCheung/omarchy-mihomo-plugin: Omarchy status-bar plugin for a standalone mihomo core · GitHub"
[2]: https://github.com/ZainCheung/omarchy-mihomo-plugin/blob/main/manifest.json "omarchy-mihomo-plugin/manifest.json at main · ZainCheung/omarchy-mihomo-plugin · GitHub"
[3]: https://github.com/Clash-Verge-rev/clash-verge-rev?utm_source=chatgpt.com "GitHub - clash-verge-rev/clash-verge-rev: A modern GUI client based on Tauri, designed to run in Windows, macOS and Linux for tailored proxy experience · GitHub"
[4]: https://github.com/MetaCubeX/mihomo/issues/3063?utm_source=chatgpt.com "[Bug] 两个`PORT`规则没有正确处理端口范围格式 · Issue #3063 · MetaCubeX/mihomo · GitHub"
