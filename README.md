# XChat 多房间内网聊天（0.4.0）

Go 服务端 + Windows x64 TUI 客户端，完整源码、测试、构建及部署脚本均在仓库。

本分支从 `origin/main` 开发：**消息撤回、TLS 网络加密、@成员、重新实现的颜文字搜索与发送**。保留任意口令建房、加密存储、多行编辑与客户端切换房间。新功能需要客户端和多房间服务端一起升级；旧版单房间服务及数据库不在本次升级范围。

## 使用客户端

解压 dist/xchat-rooms-windows-amd64-0.4.0.zip，运行 xchat-rooms.exe。源码默认连接本机 WSS 测试地址；实际内网地址通过管理员私下提供的构建包或 --server 指定。本文 chat.example.invalid 是占位地址。

请完整解压并保留 `terminal` 目录。Windows 10 2004（内部版本 19041）及以上，双击无参数启动时会使用包内的独立 Windows Terminal 和后备字体，避免传统控制台“新宋体”缺字导致颜文字显示为方框。无需安装，不改变系统默认终端。通过命令行传参、从现有终端运行或单独复制 exe 时保留当前终端；要显示全部特殊字符，需在支持字体回退的终端中运行。

    .\xchat-rooms.exe --server wss://chat.example.invalid:18081/ws
    .\xchat-rooms.exe --server wss://chat.example.invalid:18081/ws --tls-ca company-ca.pem
    .\xchat-rooms.exe --version
    .\xchat-rooms.exe --kaomoji config/kaomoji.json

输入昵称按 Enter，再输入房间口令按 Enter。相同口令进入相同房间，不存在则自动创建。无需账号、无需管理员预建房间。口令精确匹配且区分大小写与空格，1–256 个字符，不允许全空白及控制字符。不同口令就是不同房间，输错口令可能进入一个新房间。

| 操作 | 快捷键 |
| --- | --- |
| 昵称 / 口令切换 | Tab / Shift+Tab |
| 继续 / 进入 / 发送消息 | Enter |
| 换行 | Shift+Enter（Windows 原生客户端）；Ctrl+J 兼容换行 |
| 粘贴多行草稿 | 客户端 Ctrl+V（推荐），Shift+Insert |
| 编辑草稿 | 方向键、Home / End |
| 浏览历史 | PgUp / PgDn |
| 回到最新 / 已加载历史顶部 | Ctrl+End / Ctrl+Home |
| 切换房间 | F2；窗口内 Enter 确认、Esc 取消 |
| 选择颜文字 | F3；输入关键词搜索，Tab / Shift+Tab 切换分类，↑↓ 选择 |
| 插入 / 单独发送颜文字 | 选择器内 Enter 插入草稿，Ctrl+S 单独发送，Esc 取消 |
| @成员 | 输入独立的 @ 或 F4；搜索、↑↓ 选择、Enter 插入、Esc 取消 |
| 撤回本人消息 | F5；↑↓ 选择，Enter 确认后再次 Enter 撤回，Esc 取消 |
| 退出 | Ctrl+C |

正文最多 2000 个 Unicode 字符（包含换行），支持 LF 多行；昵称最多 20 字符且不能换行。同一房间内在线昵称不能重复，不同房间可以同名。口令、历史消息、在线名单不向其他房间公开。口令本身不作为房间名称显示。

**多行粘贴请优先使用客户端 Ctrl+V**，一次读入系统剪贴板，不发送草稿。某些终端的右键/外层粘贴会把 CR 当作真实 Enter；不要用这类粘贴方式输入多行。Windows 原生控制台保留 Shift 修饰键；其他终端若无法区分 Shift+Enter，可用 Ctrl+J。

断线自动重连，按房间补齐消息、保留草稿。发送未确认时显示结果未知，不自动重发以避免重复。首次显示最近 100 条，更早消息分页加载。无需退出客户端，可通过 F2 切换房间。

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

客户端显式传入 `--kaomoji config/kaomoji.json` 时使用个人本地目录并停止远程获取；发布包附带的文件仅是自定义示例，不会自动读取。颜文字以普通文本消息发送，不涉及图片、贴图或文件上传。

## 客户端内切换房间

1. 聊天中按 **F2** 打开切换窗口。口令默认空白且显示星号，昵称自动带入；Tab 可切换输入项，昵称允许修改。
2. 输入目标房间口令，按 Enter。若有未发送草稿，需要再次按 Enter 确认；**只有切换成功才丢弃旧草稿**。
3. 客户端先建立目标连接并同步历史，再退出原房间；目标昵称占用、连接失败或 15 秒超时，都保留原房间及草稿，允许修改后重试。
4. Esc 随时取消切换，返回原房间。连接过程中不重复提交，也不会把窗口内输入发成聊天消息。

有未确认消息时先等待发送结果；发送失败或断线导致结果未知，会提示再次确认，不自动重发。相同口令及相同昵称只提示“已在当前房间”，不重新连接。切换成功后聊天记录、在线名单、昵称、草稿、分页及滚动状态整体切换，旧连接的迟到事件不进入新房间。

仅显示和操作一个房间，不保存口令到磁盘、不提供房间列表、收藏、多标签页或后台未读。切换房间时保留 TLS 信任配置和当前进程的撤回身份。0.4.0 的撤回与 @标记需要新版多房间服务端；旧客户端不会处理撤回事件，应统一升级。

## TLS 传输加密

客户端与服务端使用 Go 标准库 TLS（最低 TLS 1.2），通过 `wss://` 加密口令、消息、成员名单和撤回请求。客户端校验证书链、有效期和主机名，拒绝重定向，不提供跳过证书校验选项。公共或已安装到系统的 CA 无需额外参数；公司内部 CA 用 `--tls-ca company-ca.pem` 加入信任。证书 SAN 必须包含客户端地址使用的域名或 IP。

服务端原生启用 TLS：

    ./xchat-server --listen 0.0.0.0:18081 --db data/rooms.db --encryption-key-file encryption.key --tls-cert server.crt --tls-key server.key

也可通过 `XCHAT_TLS_CERT` / `XCHAT_TLS_KEY` 环境变量配置证书。证书更新后重启该多房间服务。TLS 私钥与数据库加密主密钥用途不同，不能互相替代；不要分发服务端私钥给客户端。

远程明文连接和无证书的非回环监听默认拒绝。显式 `--allow-insecure` 仅供临时明文调试，不影响证书校验；回环 `ws://127.0.0.1` / `ws://[::1]` 可用于本机测试或 TLS 代理后端。生产安装脚本要求提供证书，以原生 TLS 启动。TLS 是客户端到服务端的传输加密，不是端到端加密。

## 存储加密与安全边界

- 昵称及正文在写入 SQLite 前使用 AES-256-GCM 认证加密；随机 nonce，认证数据绑定房间标识、消息 ID 与时间。数据库及 WAL 不写明文昵称和正文。
- 口令不保存到数据库，用独立派生密钥的 HMAC-SHA256 得到房间标识。弱口令仍可能被在线猜中，请使用足够长的随机口令并私下分享。
- 主密钥是独立随机的 32 字节，以 base64 保存在 /etc/xchat-rooms/encryption.key，root:xchat-rooms 0640。安装时仅在没有数据库和密钥的首次部署生成，绝不把密钥提交 Git。
- 启动会校验密钥，错误密钥、损坏密文或旧明文库均拒绝使用。不支持本期密钥轮换或旧库迁移。**丢失密钥将无法恢复聊天内容；不要删除或重新生成现有密钥。**
- 消息数量、时间戳、匿名房间标识和数据库结构不是加密对象。此方案不是整库加密或端到端加密；服务器可解密内容。
- 默认通过 WSS 加密网络传输，部署证书与客户端信任方式见上文。显式启用的 WS 调试连接仍为明文。
- 无个人身份验证、口令找回、私聊、文件传输、分布式或自动更新。昵称不是身份凭证。

## 构建

仅构建新版客户端及源码包（不会编译、部署或重启服务端）：

    powershell -ExecutionPolicy Bypass -File scripts/build-client.ps1 -Server wss://chat.example.invalid:18081/ws

省略 -Server 时默认连接本机 WSS 测试地址。输出为 dist/xchat-rooms-0.4.0.exe、客户端 zip 和源码 zip。仅客户端构建不升级服务端；完整启用本次功能需部署同版本服务端和有效的 TLS 证书。

需要 Go 1.26 或更高版本和 PowerShell。默认构建不包含真实服务地址：

Windows 包含微软官方 Windows Terminal 1.24.11911.0 便携发行版、其附带字体和许可文件。首次打包需访问 GitHub 下载约 11 MiB，校验固定 SHA256 后缓存到 `dist/terminal-cache`；后续构建复用缓存。后备字体配置位于 `scripts/windows-terminal/settings.json`，打包后仅作用于此客户端目录。

    go test ./...
    go vet ./...
    powershell -ExecutionPolicy Bypass -File scripts/build.ps1

私有部署包可以在本地用 -Server 指定实际地址；不要把实际地址、包或密钥提交到公共仓库。

    powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Server wss://chat.example.invalid:18081/ws

使用内部 CA 时可额外传入 `-TLSCA company-ca.pem`，将公有 CA 证书编入客户端，双击 exe 即可使用该信任配置。不要传入私钥。运行时的 `--tls-ca` 可覆盖编入的 CA，未设置两者时使用系统信任库。

输出位于 dist：版本化 Windows exe / zip、Linux 服务端 zip、完整源码 zip。新构建不会覆盖旧版 xchat.exe 或旧服务器包。

## 独立部署

仅使用 dist/rooms-server/install.sh，不运行旧版安装流程。deploy/install.sh 在本分支主动拒绝旧版部署。新包解压到独立目录后运行：

    sh install.sh '<SERVER_IP>:18081' '<CLIENT_CIDR>,127.0.0.0/8' /path/to/server.crt /path/to/server.key

先确认所选端口空闲，使用可信内网绑定地址与来源白名单，并准备 CA 签发且与连接地址匹配的证书。安装依赖 Linux systemd、Python 3.7+、标准账户/文件管理命令。已有 `/etc/xchat-rooms/tls.crt` 和 `tls.key` 时可省略后两个参数。

| 项目 | 新实例 |
| --- | --- |
| systemd 服务 | xchat-rooms.service |
| 程序 | /opt/xchat-rooms/xchat-server |
| 配置及加密密钥 | /etc/xchat-rooms/ |
| 数据库 | /var/lib/xchat-rooms/rooms.db |
| 私有管理 socket | /run/xchat-rooms/admin.sock |
| 定时器 | xchat-rooms-cleanup.timer |
| 示例端口 | 18081 |

所有路径、账户及单元均独立于旧服务。安装脚本只启用/重启多房间实例及其定时器。已有监听地址、来源白名单和数据库密钥保留，TLS 证书复制到 `/etc/xchat-rooms/tls.crt` / `tls.key`（root:xchat-rooms 0640），并更新 `server.env` 中两个 TLS 路径。证书源文件也应妥善保护。

    systemctl status xchat-rooms --no-pager
    journalctl -u xchat-rooms -n 50 --no-pager
    curl --cacert company-ca.pem https://chat.example.invalid:18081/healthz

健康检查请使用新服务实际监听地址。不会自动修改防火墙；需要时仅允许指定公司网段访问新端口，不改变原端口规则。

## 每日与手动清理

每天北京时间 **00:00** 事务性删除新实例的**全部房间及全部聊天记录**，不是只删当天活跃房间。通知所有在线客户端清空历史，保留草稿及连接；继续发送时按原房间标识重新建房。消息 ID 不重复使用，同步代次变化防止旧历史重新出现。

定时器调用与手动执行相同的 Python 脚本，通过新实例私有 socket 清理。公开 HTTP/WebSocket 不提供管理接口。

    sudo python3 /opt/xchat-rooms/clear_history.py
    sudo python3 /opt/xchat-rooms/clear_history.py --yes

仓库脚本同样可手动执行：

    sudo python3 deploy/clear_history.py

默认目标为 /run/xchat-rooms/admin.sock。高级用法允许 --socket 或 XCHAT_ROOMS_ADMIN_SOCKET 指定测试实例路径；**不要指向旧实例**。交互要求输入 CLEAR；失败不自动重试。

    systemctl list-timers xchat-rooms-cleanup.timer --no-pager
    journalctl -u xchat-rooms-cleanup.service -n 50 --no-pager

时间可配置：systemctl edit xchat-rooms-cleanup.timer，写入下面内容（例：02:30）：

    [Timer]
    OnCalendar=
    OnCalendar=*-*-* 02:30:00 Asia/Shanghai

然后 systemctl daemon-reload 并 systemctl restart xchat-rooms-cleanup.timer。Persistent=false：停机错过的清理不补跑。关闭定时清理用 systemctl disable --now xchat-rooms-cleanup.timer。

清理属于逻辑删除，不承诺物理擦除或缩小文件；备份不自动清理，需另行管理。首次交付前的验证仅清理新实例测试数据。

## 备份与回退

加密数据库和主密钥必须分别安全备份，主密钥丢失无法恢复。运行中不要单独复制 rooms.db，SQLite WAL 可能含未合并记录。可停**新实例 xchat-rooms**后成套备份其数据目录，备份结束只启动新实例；不得为此操作旧服务。

部署前备份多房间数据库和原有密钥。当前加密库可直接升级，无需修改消息表结构；新增属性仍保存在加密正文内。回退至不支持撤回的版本可能无法正确显示撤回标记，应使用升级前的独立备份测试回退。旧单房间服务和数据库保持独立，不要把加密库交给旧单房间程序打开。

## 测试与源码

    go test ./... -count=1
    go vet ./...
    python3 -m unittest discover -s deploy -p 'test_*.py'

Linux 可运行 go test -race ./...。XCHAT_EXE 指向新 exe 时启用 Windows ConPTY 测试。部署冒烟需显式设置 XCHAT_SMOKE_ADDRESS、XCHAT_SMOKE_KEY、XCHAT_SMOKE_MARKER，只能指向获准测试的新服务。

- internal/securestore：加密 SQLite、房间索引、密钥校验。
- internal/server：房间隔离、消息同步、全量清理。
- internal/tui、internal/terminal：多行编辑、Windows 原生按键。
- internal/kaomoji：服务端初始目录、共享格式解析、关键词与校验。
- internal/client、internal/protocol：通信协议、重连与正文校验。
- deploy/rooms、deploy/clear_history.py：独立部署、定时与手动清理。
- internal/store、internal/accesskey 与旧部署单元用于旧版回归参考，不能用于新实例的明文存储部署。

当前设计与实施计划见 docs/superpowers 下 2026-09-17 文档；先前验收文档是历史版本记录，不是本分支部署指南。

0.4.0 功能实现与验收见 `docs/verification-chat-features-0.4.0.md`。

多房间服务验收见 docs/verification-rooms-2026-09-17.md；客户端切换验收见 docs/verification-room-switch-2026-09-17.md；0.3.2 界面与 F2 修复验收见 docs/verification-client-ui-0.3.2.md。

