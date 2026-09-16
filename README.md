# XChat 内网命令行聊天室

Go 服务端 + Windows TUI 客户端。输入昵称和共享密钥进入一个公共聊天室，消息保存在服务端 SQLite 数据库。**完整源码、测试、构建和部署脚本均在此仓库**。

## 使用客户端

将 dist/xchat-windows-amd64.zip 解压到任意目录，在 Windows Terminal 或 PowerShell 中执行：

    .\xchat.exe

默认连接 ws://192.168.33.216:18080/ws（dev216）。输入昵称按 Enter，再输入密钥按 Enter。密钥由管理员单独提供，输入时显示为星号；Tab 可切换输入项。客户端为 Windows x64 程序，不需要安装 Go；需网络可达公司内网服务器。建议用支持中文的终端字体。

    .\xchat.exe --server ws://192.168.33.216:18080/ws
    .\xchat.exe --version

| 操作 | 快捷键 |
| --- | --- |
| 切换昵称和密钥 | Tab / Shift+Tab |
| 继续 / 进入聊天室 / 发送单行消息 | Enter |
| 浏览消息 / 到顶部加载更早消息 | PgUp、PgDn |
| 返回最新消息 | Ctrl+End |
| 跳到已加载历史顶部 | Ctrl+Home |
| 退出 | Ctrl+C |

可用终端粘贴快捷键输入中文。首次显示最近 100 条，更早记录逐页加载。窗口较窄时隐藏在线列表。当前在线昵称不可重复，离线后昵称可复用。正文最多 2,000 个 Unicode 字符，昵称最多 20 个，不接受控制字符。

断线自动重连，保留草稿并补齐离线消息。连接断开时未确认的消息显示“结果未知”，**不会自动重发**；请核对历史再决定是否手动发送。服务端先保存再广播，保存失败会提示。客户端退出后不保存本地聊天记录。

## 从源码构建

要求 Go 1.26 或更新版本。依赖版本锁定在 go.mod/go.sum。

    go mod download
    go test ./...
    go vet ./...
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1

指定默认服务器：

    powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Server ws://192.168.33.216:18080/ws -Version 0.2.1

产物：
- dist/xchat.exe：Windows amd64 客户端。
- dist/xchat-windows-amd64.zip：客户端及说明书。
- dist/xchat-server-linux-amd64：Linux amd64 服务端，无 CGO 运行时依赖。
- dist/xchat-source.zip：完整源码、测试、依赖清单、文档和脚本，不包含聊天数据库。

Linux 也可以直接构建：

    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dist/xchat.exe ./cmd/xchat
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/xchat-server-linux-amd64 ./cmd/xchat-server

## 本地开发

先在本机创建 access.key 文件，仅包含测试用共享密钥；不要提交密钥文件（*.key 已忽略）。Linux 上将文件权限设为 0600。

    go run ./cmd/xchat-server --listen 127.0.0.1:18080 --db data/chat.db --key-file access.key
    go run ./cmd/xchat --server ws://127.0.0.1:18080/ws

服务器参数：--listen、--db、--allow-cidr、--key-file，以及可选 --admin-socket。密钥未配置、为空或在 Linux 上允许其他用户读取时，服务拒绝启动。默认只监听 127.0.0.1:18080；默认来源白名单为回环地址和 RFC1918 私网。来源校验使用实际 TCP 对端地址，忽略 X-Forwarded-For。

## 部署到 dev216

目标已检查为 Kylin V10 Linux x86_64，支持 systemd。安装前确认 18080 未被占用，将下列文件上传到同一临时目录：

- dist/xchat-server-linux-amd64
- deploy/xchat.service
- deploy/install.sh
- deploy/cleanup.sh
- deploy/xchat-cleanup.service
- deploy/xchat-cleanup.timer

首次安装前，以 root 创建 /etc/xchat 目录，使用编辑器写入 /etc/xchat/access.key 并设置 0600 权限；不要把密钥放在命令参数或终端日志中。升级安装保留已有文件。安装脚本最终设置 root:xchat 所有权及 0640 权限。

以 root 执行：

    sh install.sh 192.168.33.216:18080 192.168.0.0/16,127.0.0.0/8

脚本创建 xchat 系统用户，将程序放在 /opt/xchat/xchat-server，数据库放在 /var/lib/xchat/chat.db，配置放在 /etc/xchat/server.env，服务注册为 xchat.service。开启开机启动并启动服务，**不重启主机、不修改系统防火墙、不覆盖历史数据库**。升级时保留前一个程序为 xchat-server.previous，已有 server.env 和 access.key 不会被覆盖。升级到 0.2.0 后旧客户端无法进入，需同时更新客户端。

    systemctl status xchat --no-pager
    systemctl is-enabled xchat
    journalctl -u xchat -n 100 --no-pager
    curl http://192.168.33.216:18080/healthz

默认部署只监听指定内网 IP，应用层只允许 192.168.0.0/16 和回环来源。如果公司客户端位于其他内网网段，管理员需编辑 /etc/xchat/server.env 的 XCHAT_ALLOW_CIDR，再重启服务。若启用了主机或网络防火墙，应按公司策略仅向所需内网网段开放 TCP 18080；不要直接全网开放。

    systemctl restart xchat
    systemctl stop xchat

健康检查只返回服务与数据库是否可用，不返回聊天内容。不要将已启用开机启动误认为已实际重启主机验证。

## 密钥配置

密钥保存在 /etc/xchat/access.key，不包含在客户端或 GitHub 仓库中。用管理员编辑器更改文件后，保持 root:xchat、0640 权限并重启 xchat 服务。现有客户端在重连时需输入新的密钥；客户端只在本次进程内保存密钥，不写本地配置。不要直接把密钥作为服务端命令行参数。

## 每日清理

默认每天 **北京时间 00:00** 运行 xchat-cleanup.timer，调用 /opt/xchat/cleanup.sh 清空全部聊天记录。清理在 SQLite 事务中执行，不删除数据库文件、不重启服务；在线客户端收到清理事件后刷新历史，草稿保留，仍可继续发送。

    systemctl status xchat-cleanup.timer --no-pager
    systemctl list-timers xchat-cleanup.timer --no-pager
    journalctl -u xchat-cleanup.service -n 50 --no-pager

清理时间通过 systemd timer drop-in 配置。执行 systemctl edit xchat-cleanup.timer，例如改为每天北京时间 02:30：

    [Timer]
    OnCalendar=
    OnCalendar=*-*-* 02:30:00 Asia/Shanghai

第一条空 OnCalendar 必须保留，用于清除原计划。保存后执行：

    systemctl daemon-reload
    systemctl restart xchat-cleanup.timer
    systemctl list-timers xchat-cleanup.timer --no-pager

timer 使用 Persistent=false，停机期间错过的清理不会在启动时补跑，以免意外清空新消息；下一次计划时间照常执行。关闭定时清理用 systemctl disable --now xchat-cleanup.timer。

**手工清空不可恢复，请先确认并备份**：管理员可以运行 systemctl start xchat-cleanup.service 或 /opt/xchat/cleanup.sh。脚本通过仅本机可访问、权限 0600 的 /run/xchat/admin.sock 操作，不在公网 HTTP 或聊天协议上暴露清理入口。返回非 2xx 时脚本失败，失败原因写入日志。

清理删除逻辑聊天记录，不承诺物理介质安全擦除或立即缩小 SQLite 文件；历史备份也不会自动删除，需管理员管理。

## 备份、恢复与升级

聊天历史每日零点自动清空；如需保留，请在清理前备份。仍需关注 /var/lib/xchat 的磁盘占用。默认采用停服备份：

    systemctl stop xchat
    tar -C /var/lib -czf /安全备份目录/xchat-backup.tar.gz xchat
    systemctl start xchat

将示例备份路径替换为真实的受限目录，备份包含敏感聊天内容。不要在服务运行时只复制 chat.db，因为 WAL 可能包含尚未检查点合并的数据。

恢复时先停服，保留当前数据目录作为回退，将完整备份恢复到 /var/lib/xchat，设置 xchat:xchat 所有权，再启动服务。必须成套恢复数据库及备份中的相关文件，不要把旧 WAL 与新数据库混用。恢复到更早快照后应让客户端退出重进；删除整个数据库重建会产生新实例 ID，客户端自动清空旧游标。

升级使用同一安装脚本，数据库与配置保留。若需回退程序，停止服务后将 /opt/xchat/xchat-server.previous 复制回 xchat-server，再启动；涉及未来数据库 schema 变更时应先查升级说明，不能盲目回退。

## 故障排查与安全边界

- 无法连接：确认内网/VPN、IP 与端口、服务状态、应用来源白名单以及网络防火墙。
- 昵称占用：换名或等待旧连接心跳超时释放；默认心跳每 20 秒，超时检测最多约 60 秒。
- 保存失败：检查磁盘空间、数据库目录权限与服务日志；未保存消息不会广播。
- Windows 中文显示异常：使用 UTF-8 的现代终端及中文字体；不同输入法仍需在实际电脑上验收。
- 使用共享密钥控制进入权限，但不提供个人账号或身份验证；持有密钥且来自允许网段的连接者可读取当前历史，昵称可被他人复用。
- 默认 ws 是明文，密钥与聊天内容都可能被链路窃听，只用于可信内网。需要加密时，应由公司 TLS 入口代理提供 wss，并保持后端端口仅代理可达；客户端已支持 wss 地址。
- 不做私聊、文件、撤回、多房间、分布式或自动更新。客户端长时间加载大量历史会增加内存使用。

## 验收记录

当前版本验收见 docs/verification-access-key-cleanup.md；初版记录见 docs/verification-2026-09-16.md。可选真实部署测试默认跳过，显式启用后会写入一条标记消息；Windows 伪终端测试使用本机临时数据库，不影响公共历史。

## 源码结构

    cmd/xchat/           Windows TUI 入口
    cmd/xchat-server/    服务端入口
    internal/accesskey/ 密钥文件读取与校验
    internal/admin/     受限 Unix socket 清理接口
    internal/protocol/  JSON 协议与文本校验
    internal/store/     SQLite 持久化和分页
    internal/server/    聊天会话、广播、同步、网络白名单
    internal/client/    连接、重连、历史同步、发送确认
    internal/tui/       TUI 交互和布局
    deploy/             systemd 单元与安装脚本
    scripts/            构建和打包
    docs/superpowers/   设计及实施计划

测试覆盖中文与控制字符、205 条游标分页和重开持久化、重名加入、广播与写入失败、同步期间实时消息衔接、服务重启自动补齐、最大中文历史页、窄屏布局及 TUI 启停。Linux 环境具备 C 编译器时可运行 go test -race ./...。
