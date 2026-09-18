# XChat 多房间内网聊天（客户端 0.3.2）

Go 服务端 + Windows x64 TUI 客户端，完整源码、测试、构建及部署脚本均在仓库。

本分支基于既有功能开发：**任意房间口令建房、加密聊天记录、多行编辑、内置颜文字、独立部署**。旧版单房间服务继续运行，不升级、不迁移、不清空旧版数据库。

## 使用客户端

解压 dist/xchat-rooms-windows-amd64-0.3.2.zip，运行 xchat-rooms.exe。源码默认连接本机测试地址；实际内网地址通过管理员私下提供的构建包或 --server 指定。本文 chat.example.invalid 是占位地址。

    .\xchat-rooms.exe --server ws://chat.example.invalid:18081/ws
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
| 选择颜文字 | F3；窗口内 Enter 插入、Esc 取消 |
| 退出 | Ctrl+C |

正文最多 2000 个 Unicode 字符（包含换行），支持 LF 多行；昵称最多 20 字符且不能换行。同一房间内在线昵称不能重复，不同房间可以同名。口令、历史消息、在线名单不向其他房间公开。口令本身不作为房间名称显示。

**多行粘贴请优先使用客户端 Ctrl+V**，一次读入系统剪贴板，不发送草稿。某些终端的右键/外层粘贴会把 CR 当作真实 Enter；不要用这类粘贴方式输入多行。Windows 原生控制台保留 Shift 修饰键；其他终端若无法区分 Shift+Enter，可用 Ctrl+J。

断线自动重连，按房间补齐消息、保留草稿。发送未确认时显示结果未知，不自动重发以避免重复。首次显示最近 100 条，更早消息分页加载。无需退出客户端，可通过 F2 切换房间。

## 内置颜文字

聊天中输入 **F3** 打开内置颜文字选择器，用方向键选择、Tab / ←→ 切换分类，Enter 将选中颜文字插入当前草稿光标处，Esc 取消。颜文字就是普通消息正文，不自动发送，不修改服务端或协议。

颜文字目录不再写死在代码中，而是从 JSON 配置文件加载。客户端默认读取当前工作目录下的 `config/kaomoji.json`（发布包解压后与 `xchat-rooms.exe` 同目录），可通过 `--kaomoji` 参数指定其他路径；文件缺失或格式非法时客户端会拒绝启动并提示错误。修改该文件即可增删分类与条目，无需重新编译。

## 客户端内切换房间

1. 聊天中按 **F2** 打开切换窗口。口令默认空白且显示星号，昵称自动带入；Tab 可切换输入项，昵称允许修改。
2. 输入目标房间口令，按 Enter。若有未发送草稿，需要再次按 Enter 确认；**只有切换成功才丢弃旧草稿**。
3. 客户端先建立目标连接并同步历史，再退出原房间；目标昵称占用、连接失败或 15 秒超时，都保留原房间及草稿，允许修改后重试。
4. Esc 随时取消切换，返回原房间。连接过程中不重复提交，也不会把窗口内输入发成聊天消息。

有未确认消息时先等待发送结果；发送失败或断线导致结果未知，会提示再次确认，不自动重发。相同口令及相同昵称只提示“已在当前房间”，不重新连接。切换成功后聊天记录、在线名单、昵称、草稿、分页及滚动状态整体切换，旧连接的迟到事件不进入新房间。

仅显示和操作一个房间，不保存口令到磁盘、不提供房间列表、收藏、多标签页或后台未读。客户端 0.3.2 可直接连接已部署的多房间服务，不需要升级或重启服务端；不适用于旧版单房间服务。

## 存储加密与安全边界

- 昵称及正文在写入 SQLite 前使用 AES-256-GCM 认证加密；随机 nonce，认证数据绑定房间标识、消息 ID 与时间。数据库及 WAL 不写明文昵称和正文。
- 口令不保存到数据库，用独立派生密钥的 HMAC-SHA256 得到房间标识。弱口令仍可能被在线猜中，请使用足够长的随机口令并私下分享。
- 主密钥是独立随机的 32 字节，以 base64 保存在 /etc/xchat-rooms/encryption.key，root:xchat-rooms 0640。安装时仅在没有数据库和密钥的首次部署生成，绝不把密钥提交 Git。
- 启动会校验密钥，错误密钥、损坏密文或旧明文库均拒绝使用。不支持本期密钥轮换或旧库迁移。**丢失密钥将无法恢复聊天内容；不要删除或重新生成现有密钥。**
- 消息数量、时间戳、匿名房间标识和数据库结构不是加密对象。此方案不是整库加密或端到端加密；服务器可解密内容。
- ws 链路仍是明文，只适用于可信内网；如需传输加密，部署公司 TLS 反向代理并使用 wss，后端只允许代理访问。
- 无个人身份验证、口令找回、私聊、文件传输、分布式或自动更新。昵称不是身份凭证。

## 构建

仅构建新版客户端及源码包（不会编译、部署或重启服务端）：

    powershell -ExecutionPolicy Bypass -File scripts/build-client.ps1 -Server ws://chat.example.invalid:18081/ws

省略 -Server 时默认连接本机测试地址。输出为 dist/xchat-rooms-0.3.2.exe、客户端 zip 和源码 zip，不覆盖 0.3.0 发布包。下述完整构建与服务端部署步骤仅供首次安装参考，本次客户端升级无需执行。

需要 Go 1.26 或更高版本和 PowerShell。默认构建不包含真实服务地址：

    go test ./...
    go vet ./...
    powershell -ExecutionPolicy Bypass -File scripts/build.ps1

私有部署包可以在本地用 -Server 指定实际地址；不要把实际地址、包或密钥提交到公共仓库。

    powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Server ws://chat.example.invalid:18081/ws

输出位于 dist：版本化 Windows exe / zip、Linux 服务端 zip、完整源码 zip。新构建不会覆盖旧版 xchat.exe 或旧服务器包。

## 独立部署

仅使用 dist/rooms-server/install.sh，不运行旧版安装流程。deploy/install.sh 在本分支主动拒绝旧版部署。新包解压到独立目录后运行：

    sh install.sh '<SERVER_IP>:18081' '<CLIENT_CIDR>,127.0.0.0/8'

先确认所选端口空闲，使用可信内网绑定地址与来源白名单。安装依赖 Linux systemd、Python 3.7+、标准账户/文件管理命令。

| 项目 | 新实例 |
| --- | --- |
| systemd 服务 | xchat-rooms.service |
| 程序 | /opt/xchat-rooms/xchat-server |
| 配置及加密密钥 | /etc/xchat-rooms/ |
| 数据库 | /var/lib/xchat-rooms/rooms.db |
| 私有管理 socket | /run/xchat-rooms/admin.sock |
| 定时器 | xchat-rooms-cleanup.timer |
| 示例端口 | 18081 |

所有路径、账户及单元均独立于旧服务。安装脚本只启用/重启新实例及其定时器，绝不操作旧服务。配置已存在时保留；修改新实例配置后只重启 xchat-rooms。

    systemctl status xchat-rooms --no-pager
    journalctl -u xchat-rooms -n 50 --no-pager
    curl http://127.0.0.1:18081/healthz

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

新功能回退可以停止新实例及其定时器，让用户继续使用旧客户端；旧服务和旧数据库一直保持独立。不要把新库交给旧版程序打开。

## 测试与源码

    go test ./... -count=1
    go vet ./...
    python3 -m unittest discover -s deploy -p 'test_*.py'

Linux 可运行 go test -race ./...。XCHAT_EXE 指向新 exe 时启用 Windows ConPTY 测试。部署冒烟需显式设置 XCHAT_SMOKE_ADDRESS、XCHAT_SMOKE_KEY、XCHAT_SMOKE_MARKER，只能指向获准测试的新服务。

- internal/securestore：加密 SQLite、房间索引、密钥校验。
- internal/server：房间隔离、消息同步、全量清理。
- internal/tui、internal/terminal：多行编辑、Windows 原生按键。
- internal/kaomoji：内置分类颜文字目录及校验。
- internal/client、internal/protocol：通信协议、重连与正文校验。
- deploy/rooms、deploy/clear_history.py：独立部署、定时与手动清理。
- internal/store、internal/accesskey 与旧部署单元用于旧版回归参考，不能用于新实例的明文存储部署。

当前设计与实施计划见 docs/superpowers 下 2026-09-17 文档；先前验收文档是历史版本记录，不是本分支部署指南。

多房间服务验收见 docs/verification-rooms-2026-09-17.md；客户端切换验收见 docs/verification-room-switch-2026-09-17.md；0.3.2 界面与 F2 修复验收见 docs/verification-client-ui-0.3.2.md。

