# SpaceChat 多房间内网聊天（0.4.0）

SpaceChat 是 Go 服务端与终端客户端组成的多房间聊天程序，支持 Windows amd64、Linux amd64，以及由当前 SpaceChat 服务端下发的客户端在线更新。

0.4.0 包含**消息撤回、TLS 网络加密、@成员、重新实现的颜文字搜索与发送**，并保留任意口令建房、加密存储、多行编辑与客户端切换房间。新功能需要客户端和多房间服务端一起升级；旧版单房间服务及数据库不在本次升级范围。

客户端完成一次用户级安装后，可以在新终端直接运行 `spacechat`。后续版本由所连接的 SpaceChat 服务端提供，不依赖其他下载地址，也不要求管理员权限。

## 客户端首次安装

服务端启用首装目录后，用户可以直接下载安装。示例地址 `chat.example.invalid` 仅为占位符。

Linux amd64 一键安装：

```sh
curl -fsSL https://chat.example.invalid/install/linux | sh
```

Windows amd64 一键安装：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "Invoke-RestMethod 'https://chat.example.invalid/install/windows' | Invoke-Expression"
```

引导脚本从同一服务端下载签名发布流程生成的安装包，核对 SHA-256 后才解压和调用平台安装器。首次安装脚本本身是信任入口；正式环境应使用 HTTPS。需要先审查 Linux 脚本时可执行：

```sh
curl -fLo install-spacechat.sh https://chat.example.invalid/install/linux
less install-spacechat.sh
sh install-spacechat.sh
```

也可以继续离线分发对应平台安装包：

Windows：解压 `spacechat-windows-amd64-0.4.0.zip`，在该目录运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1 -Server wss://chat.example.invalid/ws -Version 0.4.0
```

Linux：解压 `spacechat-linux-amd64-0.4.0.zip`，在该目录运行：

```sh
sh ./install.sh 'wss://chat.example.invalid/ws' '0.4.0'
```

## 消息撤回与 @成员

F5 列出当前客户端发送、且在两分钟内的可撤回消息。确认后，房间中的新版客户端显示“消息已撤回”，服务端用加密撤回标记替换正文并删除该消息的 @名单。撤回按服务端时间判定，不依赖昵称认领消息；别人使用相同昵称不能撤回你的消息。

客户端身份只保存在当前进程内，自动重连及 F2 切换房间时保留；退出客户端后无法重新认领原来的消息。旧版本发送的消息没有身份信息，不能撤回。离线期间发生撤回时，重连会重新加载该房间最近 100 条历史，防止缓存恢复已撤回正文；更早记录仍可分页查看。撤回是逻辑操作，无法收回他人已复制的内容，也不承诺物理擦除数据库旧页和备份。

输入独立的 `@` 或按 F4 打开当前房间在线成员选择器。Enter 插入后仍可编辑草稿，再按 Enter 发送。也可直接输入 `@小明`；含空格、标点的昵称使用选择器生成的 `@["Bob Smith"]`。服务端按发送时的本房间在线名单识别并去重；邮箱中的 @、相似昵称、其他房间成员不触发提醒。被提及者看到“提到了你”提示和消息旁的 `@你` 标记，历史中保留标记。

## 颜文字

聊天中按 **F3** 打开重新实现的选择器，输入中文或英文关键词搜索（如“开心”、`hello`），Tab / Shift+Tab 切换分类，↑↓ 选择、PgUp / PgDn 翻页。Enter 在当前草稿光标处插入完整颜文字；超出 2000 字符时拒绝插入并保留草稿。**Ctrl+S 单独发送选中颜文字，保留原草稿**。断线或发送队列不可用时保留选择，Esc 取消。

默认从服务端 `GET /api/kaomoji` 获取目录，包含软萌、搞怪、经典等 10 类 72 项。客户端连接成功、重连、换房及打开 F3 时检查更新，使用 ETag 避免重复下载；最近一次有效结果缓存在当前进程中。加载不阻塞聊天，失败时保留已有目录和草稿，首次获取失败会显示提示。GET 使用与聊天相同的地址、TLS 信任和来源白名单，不发送房间口令，也不接受重定向。

服务端安装时创建 `/etc/xchat-rooms/kaomoji.json`，升级保留已有文件。可通过服务端 `--kaomoji` 或 `XCHAT_KAOMOJI` 指定其他路径；没有指定时使用服务端内置初始目录 `internal/kaomoji/defaults.json`。修改已配置的文件后，下次 GET 自动重载，**无需重启服务或重新编译客户端**。建议先编辑临时文件，校验后在同一目录重命名替换，保持文件可由 `xchat-rooms` 用户读取。运行中的错误配置保留上一次有效目录并记录日志；启动时配置无效则拒绝启动。

配置格式示例：

```json
{"version":1,"categories":[{"id":"cute","name":"软萌","items":[{"text":"(｡•ᴗ•｡)","keywords":["可爱","cute"]}]}]}
```

`version` 是格式版本。目录最大 1 MiB、64 个分类、4096 个表情，每项最多 32 个搜索词；分类名、文本和搜索词必须是有效单行文本，同一分类内表情不重复。目录对允许连接的客户端共享，不能放私密内容。公开接口只读，配置通过服务器文件管理。

客户端显式传入 `--kaomoji config/kaomoji.json` 时使用个人本地目录并停止远程获取；本地目录文件需另行提供。颜文字以普通文本消息发送，不涉及图片、贴图或文件上传。

## 客户端内切换房间

安装后打开新终端：

```text
spacechat --version
spacechat
```

仅显示和操作一个房间，不保存口令到磁盘、不提供房间列表、收藏、多标签页或后台未读。切换房间时保留 TLS 信任配置和当前进程的撤回身份。0.4.0 的撤回与 @标记需要新版多房间服务端；旧客户端不会处理撤回事件，应统一升级。

## TLS 传输加密

客户端与服务端使用 Go 标准库 TLS（最低 TLS 1.2），通过 `wss://` 加密口令、消息、成员名单和撤回请求。客户端校验证书链、有效期和主机名，拒绝重定向，不提供跳过证书校验选项。公共或已安装到系统的 CA 无需额外参数；公司内部 CA 用 `--tls-ca company-ca.pem` 加入信任。证书 SAN 必须包含客户端地址使用的域名或 IP。

服务端原生启用 TLS：

    ./xchat-server --listen 0.0.0.0:18081 --db data/rooms.db --encryption-key-file encryption.key --tls-cert server.crt --tls-key server.key

也可通过 `XCHAT_TLS_CERT` / `XCHAT_TLS_KEY` 环境变量配置证书。证书更新后重启该多房间服务。TLS 私钥与数据库加密主密钥用途不同，不能互相替代；不要分发服务端私钥给客户端。

远程明文连接和无证书的非回环监听默认拒绝。显式 `--allow-insecure` 仅供临时明文调试，不影响证书校验；回环 `ws://127.0.0.1` / `ws://[::1]` 可用于本机测试或 TLS 代理后端。生产安装脚本要求提供证书，以原生 TLS 启动。TLS 是客户端到服务端的传输加密，不是端到端加密。

临时连接另一台服务端时，命令行参数优先于安装配置：

```text
spacechat --server wss://other.example.invalid/ws
```

Windows 安装在 `%LOCALAPPDATA%\SpaceChat`，并只修改当前用户的 `PATH`。Linux 启动器位于 `~/.local/bin/spacechat`，配置与版本文件分别使用 XDG 配置目录和数据目录。安装器拒绝符号链接输入，先运行版本化客户端的本地自检，再原子写入配置和当前版本指针。

- 昵称及正文在写入 SQLite 前使用 AES-256-GCM 认证加密；随机 nonce，认证数据绑定房间标识、消息 ID 与时间。数据库及 WAL 不写明文昵称和正文。
- 口令不保存到数据库，用独立派生密钥的 HMAC-SHA256 得到房间标识。弱口令仍可能被在线猜中，请使用足够长的随机口令并私下分享。
- 主密钥是独立随机的 32 字节，以 base64 保存在 /etc/xchat-rooms/encryption.key，root:xchat-rooms 0640。安装时仅在没有数据库和密钥的首次部署生成，绝不把密钥提交 Git。
- 启动会校验密钥，错误密钥、损坏密文或旧明文库均拒绝使用。不支持本期密钥轮换或旧库迁移。**丢失密钥将无法恢复聊天内容；不要删除或重新生成现有密钥。**
- 消息数量、时间戳、匿名房间标识和数据库结构不是加密对象。此方案不是整库加密或端到端加密；服务器可解密内容。
- 默认通过 WSS 加密网络传输，部署证书与客户端信任方式见上文。显式启用的 WS 调试连接仍为明文。
- 无个人身份验证、口令找回、私聊、文件传输或分布式部署。昵称不是身份凭证。

## 在线更新行为

每次进入聊天界面前，客户端从当前服务端的固定接口读取已签名清单：

- 已是最新版本：直接进入程序。
- 存在兼容更新：显示 `Update / Skip`；跳过只对本次启动有效。
- 当前版本低于服务端最低版本：显示 `Update / Exit`，不能跳过。
- 下载、摘要校验、自检或启动新版失败：保留旧版本；可选更新可继续使用，强制更新只能重试或退出。

更新制品只从 `spacechat --server` 或安装配置选定的服务端下载。客户端拒绝跨主机重定向，并验证 Ed25519 清单签名、文件名、大小和 SHA-256。新版安装到独立版本目录，自检通过后才原子切换 `current`；新版无法启动时恢复旧指针。

WebSocket 加入请求会同时上报版本与平台。服务端在读取房间、历史和成员信息前执行最低版本检查，因此直接运行旧版本化程序也不能绕过强制更新。

Windows 包含微软官方 Windows Terminal 1.24.11911.0 便携发行版、其附带字体和许可文件。首次打包需访问 GitHub 下载约 11 MiB，校验固定 SHA256 后缓存到 `dist/terminal-cache`；后续构建复用缓存。后备字体配置位于 `scripts/windows-terminal/settings.json`，安装后仅作用于当前用户的 SpaceChat 目录。

## 聊天使用

输入昵称后按 Enter，再输入房间口令按 Enter。相同口令进入相同房间，不存在则自动创建。口令精确匹配且区分大小写与空格，长度为 1–256 个字符；不同口令就是不同房间。

| 操作 | 快捷键 |
| --- | --- |
| 昵称 / 口令切换 | Tab / Shift+Tab |
| 继续 / 进入 / 发送消息 | Enter |
| 换行 | Shift+Enter（Windows 原生客户端）；Ctrl+J 兼容换行 |
| 粘贴多行草稿 | Ctrl+V（推荐），Shift+Insert |
| 编辑草稿 | 方向键、Home / End |
| 浏览历史 | PgUp / PgDn |
| 回到最新 / 已加载历史顶部 | Ctrl+End / Ctrl+Home |
| 切换房间 | F2；窗口内 Enter 确认、Esc 取消 |
| 选择颜文字 | F3；输入关键词搜索，Tab / Shift+Tab 切换分类，↑↓ 选择 |
| 插入 / 单独发送颜文字 | 选择器内 Enter 插入草稿，Ctrl+S 单独发送，Esc 取消 |
| @成员 | 输入独立的 @ 或 F4；搜索、↑↓ 选择、Enter 插入、Esc 取消 |
| 撤回本人消息 | F5；↑↓ 选择，Enter 确认后再次 Enter 撤回，Esc 取消 |
| 退出 | Ctrl+C |

断线后客户端自动重连并按房间补齐消息。发送结果未知时不会自动重发，避免重复消息。F2 切房会先建立目标连接并同步历史，成功后才退出原房间；失败、昵称占用或超时均保留原房间与草稿。

## 签名构建与发布

需要 Go 1.26 或更高版本和 PowerShell。正式清单必须显式提供 Ed25519 私钥。下面用 OpenSSL 生成一次 32 字节随机种子；该文件必须离线保管，不能提交仓库、复制到服务端或发给客户端：

```sh
umask 077
openssl rand -base64 32 > spacechat-release.key
```

仅构建客户端、安装包与签名更新目录：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-client.ps1 `
  -Server wss://chat.example.invalid/ws `
  -Version 0.4.0 `
  -MinimumVersion 0.0.0 `
  -SigningKey .\spacechat-release.key
```

完整构建客户端与 Linux 服务端包：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build.ps1 `
  -Server wss://chat.example.invalid/ws `
  -Version 0.4.0 `
  -MinimumVersion 0.0.0 `
  -SigningKey .\spacechat-release.key
```

`dist` 中会生成两个平台的稳定启动器、版本化客户端、一次性安装包、签名更新目录、独立签名的首装目录以及服务端包。构建把发布公钥嵌入客户端，并把同一公钥随服务端包发布；构建日志不会输出私钥。

使用内部 CA 时可额外传入 `-TLSCA company-ca.pem`，将公有 CA 证书编入客户端。不要传入私钥。运行时的 `--tls-ca` 可覆盖编入的 CA，未设置两者时使用系统信任库。

也可以单独检查公钥或生成清单：

```text
go run ./cmd/spacechat-release public-key -private-key ./spacechat-release.key
go run ./cmd/spacechat-release manifest -version 0.4.0 -minimum 0.3.2 -private-key ./spacechat-release.key -windows ./client.exe -linux ./client -out ./updates-0.4.0
go run ./cmd/spacechat-release installers -version 0.4.0 -server wss://chat.example.invalid/ws -private-key ./spacechat-release.key -windows-package ./spacechat-windows-amd64-0.4.0.zip -linux-package ./spacechat-linux-amd64-0.4.0.zip -out ./installers-0.4.0
```

## 服务端部署

解压完整服务端包后，以 root 运行其中的安装脚本：

```sh
sh install.sh '0.0.0.0:18081' '10.0.0.0/8,127.0.0.0/8' /path/to/server.crt /path/to/server.key
```

参数依次是监听地址、允许访问的 CIDR 列表、TLS 证书和私钥。已有 `/etc/xchat-rooms/tls.crt` 与 `tls.key` 时可省略后两个参数。示例地址不是生产配置；证书 SAN 必须匹配客户端连接地址。安装依赖 Linux systemd、Python 3.7+ 和标准账户/文件管理命令。

| 项目 | 路径 |
| --- | --- |
| systemd 服务 | `xchat-rooms.service` |
| 服务端程序 | `/opt/xchat-rooms/xchat-server` |
| 已验证更新目录 | `/opt/xchat-rooms/updates` |
| 已验证首装目录 | `/opt/xchat-rooms/installers` |
| 更新公钥 | `/etc/xchat-rooms/update-public.key` |
| TLS 证书与私钥 | `/etc/xchat-rooms/tls.crt`、`/etc/xchat-rooms/tls.key` |
| 颜文字目录 | `/etc/xchat-rooms/kaomoji.json` |
| 数据库与主密钥 | `/var/lib/xchat-rooms/rooms.db`、`/etc/xchat-rooms/encryption.key` |
| 管理 socket | `/run/xchat-rooms/admin.sock` |

安装器先复制到 `updates.new` 和 `installers.new`，拒绝任何符号链接，再调用新服务端二进制的生产校验逻辑验证两份清单的签名、版本、摘要和文件大小。只有验证成功才切换更新目录、首装目录、公钥、程序和 systemd 单元；服务重启失败时恢复上一套内容。

常用检查：

```text
systemctl status xchat-rooms --no-pager
journalctl -u xchat-rooms -n 50 --no-pager
curl --cacert /path/to/company-ca.pem https://chat.example.invalid:18081/healthz
```

## 最低版本迁移与回退

首次启用在线更新时按以下顺序发布：

1. 使用 `MinimumVersion=0.0.0` 部署新服务端和清单，旧客户端暂时仍可连接。
2. 向存量用户提供最后一次人工安装包，使其切换到 `spacechat` 稳定入口。
3. 确认迁移完成后，再发布将最低版本提高到首个在线更新客户端版本的完整服务端包。

兼容发版只提高 `Version`；存在协议不兼容时才同步提高 `MinimumVersion`。最低版本不得高于清单最新版本。

回退时重新部署上一份完整服务端包，必须同时回退服务端二进制、签名更新目录、公钥和最低版本，不能只替换单个文件。已经更新到更高版本的客户端会把较旧清单视为服务端回退，不会自动降级；只要仍满足回退清单的最低版本即可继续连接。不要手工修改 `manifest.json`，任何字节变化都会使签名失效。

## 存储、安全与清理

部署前备份多房间数据库和原有密钥。当前加密库可直接升级，无需修改消息表结构；新增属性仍保存在加密正文内。回退至不支持撤回的版本可能无法正确显示撤回标记，应使用升级前的独立备份测试回退。旧单房间服务和数据库保持独立，不要把加密库交给旧单房间程序打开。

- 昵称和正文写入 SQLite 前使用 AES-256-GCM；口令通过独立派生密钥的 HMAC-SHA256 得到房间标识，不保存原文。
- 数据库主密钥位于 `/etc/xchat-rooms/encryption.key`，安装时仅在没有既有数据库时生成。丢失主密钥无法恢复聊天内容。
- 签名私钥与数据库主密钥是两类不同密钥；前者不得部署，后者不得随发布包分发。
- `ws` 与 `wss` 均受支持；需要传输加密时由部署环境提供 TLS，并使用 `wss` 地址。
- 服务端仍可解密聊天内容；本项目不是端到端加密系统，也不提供个人账号、口令找回、私聊或文件传输。

每天北京时间 00:00，`xchat-rooms-cleanup.timer` 事务性清空全部房间历史。手动清理：

```text
sudo python3 /opt/xchat-rooms/clear_history.py
sudo python3 /opt/xchat-rooms/clear_history.py --yes
```

运行中不要单独复制 SQLite 数据库；应停止新实例后成套备份数据目录与主密钥。部署和清理脚本只操作 `xchat-rooms` 实例。

- internal/securestore：加密 SQLite、房间索引、密钥校验。
- internal/server：房间隔离、消息同步、全量清理。
- internal/tui、internal/terminal：多行编辑、Windows 原生按键。
- internal/kaomoji：服务端初始目录、共享格式解析、关键词与校验。
- internal/client、internal/protocol：通信协议、重连与正文校验。
- deploy/rooms、deploy/clear_history.py：独立部署、定时与手动清理。
- internal/store、internal/accesskey 与旧部署单元用于旧版回归参考，不能用于新实例的明文存储部署。

## 验证

```text
go test ./... -count=1
go vet ./...
python3 -m unittest discover -s deploy -p 'test_*.py'
```

0.4.0 功能实现与验收见 `docs/verification-chat-features-0.4.0.md`。

多房间服务验收见 docs/verification-rooms-2026-09-17.md；客户端切换验收见 docs/verification-room-switch-2026-09-17.md；0.3.2 界面与 F2 修复验收见 docs/verification-client-ui-0.3.2.md。

Linux 可额外运行 `go test -race ./...`。主要边界：`internal/update` 负责签名、下载和事务更新，`internal/server` 负责更新目录及版本门槛，`cmd/spacechat` 是稳定入口，`cmd/xchat` 是可更新的版本化客户端。
