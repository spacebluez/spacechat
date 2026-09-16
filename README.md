# XChat 内网命令行聊天室

Go 服务端 + Windows TUI 客户端。输入昵称进入一个公共聊天室，消息保存在服务端 SQLite 数据库。**完整源码、测试、构建和部署脚本均在此仓库**。

## 使用客户端

将 dist/xchat-windows-amd64.zip 解压到任意目录，在 Windows Terminal 或 PowerShell 中执行：

    .\xchat.exe

默认连接 ws://192.168.33.216:18080/ws（dev216）。输入昵称后按 Enter。客户端为 Windows x64 程序，不需要安装 Go；需网络可达公司内网服务器。建议用支持中文的终端字体。

    .\xchat.exe --server ws://192.168.33.216:18080/ws
    .\xchat.exe --version

| 操作 | 快捷键 |
| --- | --- |
| 进入聊天室 / 发送单行消息 | Enter |
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

    powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Server ws://192.168.33.216:18080/ws -Version 0.1.0

产物：
- dist/xchat.exe：Windows amd64 客户端。
- dist/xchat-windows-amd64.zip：客户端及说明书。
- dist/xchat-server-linux-amd64：Linux amd64 服务端，无 CGO 运行时依赖。
- dist/xchat-source.zip：完整源码、测试、依赖清单、文档和脚本，不包含聊天数据库。

Linux 也可以直接构建：

    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dist/xchat.exe ./cmd/xchat
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/xchat-server-linux-amd64 ./cmd/xchat-server

## 本地开发

    go run ./cmd/xchat-server --listen 127.0.0.1:18080 --db data/chat.db
    go run ./cmd/xchat --server ws://127.0.0.1:18080/ws

服务器参数：--listen、--db、--allow-cidr。默认只监听 127.0.0.1:18080；默认来源白名单为回环地址和 RFC1918 私网。来源校验使用实际 TCP 对端地址，忽略 X-Forwarded-For。

## 部署到 dev216

目标已检查为 Kylin V10 Linux x86_64，支持 systemd。安装前确认 18080 未被占用，将下列文件上传到同一临时目录：

- dist/xchat-server-linux-amd64
- deploy/xchat.service
- deploy/install.sh

以 root 执行：

    sh install.sh 192.168.33.216:18080 192.168.0.0/16,127.0.0.0/8

脚本创建 xchat 系统用户，将程序放在 /opt/xchat/xchat-server，数据库放在 /var/lib/xchat/chat.db，配置放在 /etc/xchat/server.env，服务注册为 xchat.service。开启开机启动并启动服务，**不重启主机、不修改系统防火墙、不覆盖历史数据库**。升级时保留前一个程序为 xchat-server.previous，已有 server.env 不会被覆盖。

    systemctl status xchat --no-pager
    systemctl is-enabled xchat
    journalctl -u xchat -n 100 --no-pager
    curl http://192.168.33.216:18080/healthz

默认部署只监听指定内网 IP，应用层只允许 192.168.0.0/16 和回环来源。如果公司客户端位于其他内网网段，管理员需编辑 /etc/xchat/server.env 的 XCHAT_ALLOW_CIDR，再重启服务。若启用了主机或网络防火墙，应按公司策略仅向所需内网网段开放 TCP 18080；不要直接全网开放。

    systemctl restart xchat
    systemctl stop xchat

健康检查只返回服务与数据库是否可用，不返回聊天内容。不要将已启用开机启动误认为已实际重启主机验证。

## 备份、恢复与升级

消息不会自动过期，需定期关注 /var/lib/xchat 的磁盘占用。默认采用停服备份：

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
- 不提供账号、身份验证或传输加密；任何允许网段的连接者都可读取公共历史，昵称可被他人复用。
- 默认 ws 是明文，只用于可信内网。需要加密时，应由公司 TLS 入口代理提供 wss，并保持后端端口仅代理可达；客户端已支持 wss 地址。
- 不做私聊、文件、撤回、多房间、分布式或自动更新。客户端长时间加载大量历史会增加内存使用。

## 验收记录

实测部署、自动化测试和未执行项见 docs/verification-2026-09-16.md。可选真实部署测试默认跳过，显式启用后会写入一条标记消息；Windows 伪终端测试使用本机临时数据库，不影响公共历史。

## 源码结构

    cmd/xchat/           Windows TUI 入口
    cmd/xchat-server/    服务端入口
    internal/protocol/  JSON 协议与文本校验
    internal/store/     SQLite 持久化和分页
    internal/server/    聊天会话、广播、同步、网络白名单
    internal/client/    连接、重连、历史同步、发送确认
    internal/tui/       TUI 交互和布局
    deploy/             systemd 单元与安装脚本
    scripts/            构建和打包
    docs/superpowers/   设计及实施计划

测试覆盖中文与控制字符、205 条游标分页和重开持久化、重名加入、广播与写入失败、同步期间实时消息衔接、服务重启自动补齐、最大中文历史页、窄屏布局及 TUI 启停。Linux 环境具备 C 编译器时可运行 go test -race ./...。
